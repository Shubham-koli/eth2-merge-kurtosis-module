package nethermind
import (
	"fmt"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/module_io"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/el"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/el/el_rest_client"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/el/mining_waiter"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/service_launch_utils"
	"github.com/kurtosis-tech/kurtosis-core-api-lib/api/golang/lib/enclaves"
	"github.com/kurtosis-tech/kurtosis-core-api-lib/api/golang/lib/services"
	"github.com/kurtosis-tech/stacktrace"
	"time"
)

const (
	// The dirpath of the execution data directory on the client container
	executionDataDirpathOnClientContainer = "/execution-data"

	// The filepath of the genesis JSON file in the shared directory, relative to the shared directory root
	sharedNethermindGenesisJsonRelFilepath = "nethermind_genesis.json"

	miningRewardsAccount = "0x0000000000000000000000000000000000000001"

	rpcPortNum       uint16 = 8545
	wsPortNum        uint16 = 8546
	discoveryPortNum uint16 = 30303

	// Port IDs
	rpcPortId          = "rpc"
	wsPortId           = "ws"
	tcpDiscoveryPortId = "tcp-discovery"
	udpDiscoveryPortId = "udp-discovery"

	getNodeInfoMaxRetries         = 20
	getNodeInfoTimeBetweenRetries = 500 * time.Millisecond
)

var usedPorts = map[string]*services.PortSpec{
	rpcPortId:          services.NewPortSpec(rpcPortNum, services.PortProtocol_TCP),
	wsPortId:           services.NewPortSpec(wsPortNum, services.PortProtocol_TCP),
	tcpDiscoveryPortId: services.NewPortSpec(discoveryPortNum, services.PortProtocol_TCP),
	udpDiscoveryPortId: services.NewPortSpec(discoveryPortNum, services.PortProtocol_UDP),
}
var nethermindLogLevels = map[module_io.ParticipantLogLevel]string{
	module_io.ParticipantLogLevel_Error: "ERROR",
	module_io.ParticipantLogLevel_Warn:  "WARN",
	module_io.ParticipantLogLevel_Info:  "INFO",
	module_io.ParticipantLogLevel_Debug: "DEBUG",
	module_io.ParticipantLogLevel_Trace: "TRACE",
}

type NethermindELClientLauncher struct {
	genesisJsonFilepathOnModule string
	totalTerminalDifficulty     uint64
}

func NewNethermindELClientLauncher(genesisJsonFilepathOnModule string, totalTerminalDifficulty uint64) *NethermindELClientLauncher {
	return &NethermindELClientLauncher{genesisJsonFilepathOnModule: genesisJsonFilepathOnModule, totalTerminalDifficulty: totalTerminalDifficulty}
}

func (launcher *NethermindELClientLauncher) Launch(
	enclaveCtx *enclaves.EnclaveContext,
	serviceId services.ServiceID,
	image string,
	logLevel module_io.ParticipantLogLevel,
	bootnodeContext *el.ELClientContext,
) (resultClientCtx *el.ELClientContext, resultErr error) {
	containerConfigSupplier := launcher.getContainerConfigSupplier(image, bootnodeContext, logLevel)
	serviceCtx, err := enclaveCtx.AddService(serviceId, containerConfigSupplier)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred launching the Geth EL client with service ID '%v'", serviceId)
	}

	restClient := el_rest_client.NewELClientRESTClient(
		serviceCtx.GetPrivateIPAddress(),
		rpcPortNum,
	)

	nodeInfo, err := el.WaitForELClientAvailability(restClient, getNodeInfoMaxRetries, getNodeInfoTimeBetweenRetries)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred waiting for the EL client to become available")
	}

	miningWaiter := mining_waiter.NewMiningWaiter(restClient)
	result := el.NewELClientContext(
		nodeInfo.Enode,
		serviceCtx.GetPrivateIPAddress(),
		rpcPortNum,
		wsPortNum,
		miningWaiter,
	)

	return result, nil
}

// ====================================================================================================
//                                       Private Helper Methods
// ====================================================================================================
func (launcher *NethermindELClientLauncher) getContainerConfigSupplier(
	image string,
	bootnodeCtx *el.ELClientContext,
	logLevel module_io.ParticipantLogLevel,
) func(string, *services.SharedPath) (*services.ContainerConfig, error) {
	result := func(privateIpAddr string, sharedDir *services.SharedPath) (*services.ContainerConfig, error) {
		nethermindLogLevel, found := nethermindLogLevels[logLevel]
		if !found {
			return nil, stacktrace.NewError("No Nethermind log level defined for client log level '%v'; this is a bug in the module", logLevel)
		}

		nethermindGenesisJsonSharedPath := sharedDir.GetChildPath(sharedNethermindGenesisJsonRelFilepath)
		if err := service_launch_utils.CopyFileToSharedPath(launcher.genesisJsonFilepathOnModule, nethermindGenesisJsonSharedPath); err != nil {
			return nil, stacktrace.Propagate(
				err,
				"An error occurred copying the Nethermind genesis JSON file from '%v' into the Nethermind node being started",
				launcher.genesisJsonFilepathOnModule,
			 )
		}

		commandArgs := []string{
			"--config=kintsugi",
			"--log=" + nethermindLogLevel,
			"--datadir=" + executionDataDirpathOnClientContainer,
			"--Init.ChainSpecPath=" + nethermindGenesisJsonSharedPath.GetAbsPathOnServiceContainer(),
			"--Init.WebSocketsEnabled=true",
			"--Init.DiagnosticMode=None",
			"--JsonRpc.Enabled=true",
			"--JsonRpc.EnabledModules=net,eth,consensus,engine,admin",
			"--JsonRpc.Host=0.0.0.0",
			// TODO Set Eth isMining?
			fmt.Sprintf("--JsonRpc.Port=%v", rpcPortNum),
			fmt.Sprintf("--JsonRpc.WebSocketsPort=%v", wsPortNum),
			fmt.Sprintf("--Network.ExternalIp=%v", privateIpAddr),
			fmt.Sprintf("--Network.LocalIp=%v", privateIpAddr),
			fmt.Sprintf("--Network.DiscoveryPort=%v", discoveryPortNum),
			fmt.Sprintf("--Network.P2PPort=%v", discoveryPortNum),
			"--Merge.Enabled=true",
			fmt.Sprintf("--Merge.TerminalTotalDifficulty=%v", launcher.totalTerminalDifficulty),
			"--Merge.BlockAuthorAccount=" + miningRewardsAccount,
		}
		if bootnodeCtx != nil {
			commandArgs = append(
				commandArgs,
				"--Discovery.Bootnodes=" + bootnodeCtx.GetEnode(),
			)
		}

		containerConfig := services.NewContainerConfigBuilder(
			image,
		).WithUsedPorts(
			usedPorts,
		).WithCmdOverride(
			commandArgs,
		).Build()

		return containerConfig, nil
	}
	return result
}