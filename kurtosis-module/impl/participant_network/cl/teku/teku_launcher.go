package teku

import (
	"fmt"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/module_io"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/cl"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/cl/availability_waiter"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/cl/cl_client_rest_client"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/el"
	cl2 "github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/prelaunch_data_generator/cl_validator_keystores"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/service_launch_utils"
	"github.com/kurtosis-tech/kurtosis-core-api-lib/api/golang/lib/enclaves"
	"github.com/kurtosis-tech/kurtosis-core-api-lib/api/golang/lib/services"
	"github.com/kurtosis-tech/stacktrace"
	recursive_copy "github.com/otiai10/copy"
	"strings"
	"time"
)

const (
	tekuBinaryFilepathInImage = "/opt/teku/bin/teku"

	// The Docker container runs as the "teku" user so we can't write to root
	consensusDataDirpathOnServiceContainer = "/opt/teku/consensus-data"

	// TODO Get rid of this being hardcoded; should be shared
	validatingRewardsAccount = "0x0000000000000000000000000000000000000001"

	// Port IDs
	tcpDiscoveryPortID = "tcp-discovery"
	udpDiscoveryPortID = "udp-discovery"
	httpPortID         = "http"

	// Port nums
	discoveryPortNum uint16 = 9000
	httpPortNum             = 4000

	genesisConfigYmlRelFilepathInSharedDir = "genesis-config.yml"
	genesisSszRelFilepathInSharedDir = "genesis.ssz"

	validatorKeysDirpathRelToSharedDirRoot = "validator-keys"
	validatorSecretsDirpathRelToSharedDirRoot = "validator-secrets"

	// 1) The Teku container runs as the "teku" user
	// 2) Teku requires write access to the validator secrets directory, so it can write a lockfile into it as it uses the keys
	// 3) The module container runs as 'root'
	// With these three things combined, it means that when the module container tries to write the validator keys/secrets into
	//  the shared directory, it does so as 'root'. When Teku tries to consum the same files, it will get a failure because it
	//  doesn't have permission to write to the 'validator-secrets' directory.
	// To get around this, we copy the files AGAIN from
	destValidatorKeysDirpathInServiceContainer = "$HOME/validator-keys"
	destValidatorSecretsDirpathInServiceContainer = "$HOME/validator-secrets"

	// Teku nodes take ~35s to bring their HTTP server up
	maxNumHealthcheckRetries = 60
	timeBetweenHealthcheckRetries = 1 * time.Second

	minPeers = 1
)
var usedPorts = map[string]*services.PortSpec{
	// TODO Add metrics port
	tcpDiscoveryPortID: services.NewPortSpec(discoveryPortNum, services.PortProtocol_TCP),
	udpDiscoveryPortID: services.NewPortSpec(discoveryPortNum, services.PortProtocol_UDP),
	httpPortID:         services.NewPortSpec(httpPortNum, services.PortProtocol_TCP),
}
var tekuLogLevels = map[module_io.ParticipantLogLevel]string{
	module_io.ParticipantLogLevel_Error: "ERROR",
	module_io.ParticipantLogLevel_Warn:  "WARN",
	module_io.ParticipantLogLevel_Info:  "INFO",
	module_io.ParticipantLogLevel_Debug: "DEBUG",
	module_io.ParticipantLogLevel_Trace: "TRACE",
}

type TekuCLClientLauncher struct {
	genesisConfigYmlFilepathOnModuleContainer string
	genesisSszFilepathOnModuleContainer string
	expectedNumBeaconNodes uint32
}

func NewTekuCLClientLauncher(genesisConfigYmlFilepathOnModuleContainer string, genesisSszFilepathOnModuleContainer string, expectedNumBeaconNodes uint32) *TekuCLClientLauncher {
	return &TekuCLClientLauncher{genesisConfigYmlFilepathOnModuleContainer: genesisConfigYmlFilepathOnModuleContainer, genesisSszFilepathOnModuleContainer: genesisSszFilepathOnModuleContainer, expectedNumBeaconNodes: expectedNumBeaconNodes}
}

func (launcher *TekuCLClientLauncher) Launch(
	enclaveCtx *enclaves.EnclaveContext,
	serviceId services.ServiceID,
	image string,
	// TODO move to launcher param
	logLevel module_io.ParticipantLogLevel,
	bootnodeContext *cl.CLClientContext,
	elClientContext *el.ELClientContext,
	nodeKeystoreDirpaths *cl2.NodeTypeKeystoreDirpaths,
) (resultClientCtx *cl.CLClientContext, resultErr error) {
	containerConfigSupplier := launcher.getContainerConfigSupplier(
		image,
		bootnodeContext,
		elClientContext,
		logLevel,
		nodeKeystoreDirpaths.TekuKeysDirpath,
		nodeKeystoreDirpaths.TekuSecretsDirpath,
	)
	serviceCtx, err := enclaveCtx.AddService(serviceId, containerConfigSupplier)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred launching the Teku CL client with service ID '%v'", serviceId)
	}

	httpPort, found := serviceCtx.GetPrivatePorts()[httpPortID]
	if !found {
		return nil, stacktrace.NewError("Expected new Teku service to have port with ID '%v', but none was found", httpPortID)
	}

	restClient := cl_client_rest_client.NewCLClientRESTClient(serviceCtx.GetPrivateIPAddress(), httpPort.GetNumber())

	if err := availability_waiter.WaitForBeaconClientAvailability(restClient, maxNumHealthcheckRetries, timeBetweenHealthcheckRetries); err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred waiting for the new Teku node to become available")
	}

	// TODO add validator availability using teh validator API: https://ethereum.github.io/beacon-APIs/?urls.primaryName=v1#/ValidatorRequiredApi

	nodeIdentity, err := restClient.GetNodeIdentity()
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting the new Teku node's identity, which is necessary to retrieve its ENR")
	}

	result := cl.NewCLClientContext(
		nodeIdentity.ENR,
		serviceCtx.GetPrivateIPAddress(),
		httpPortNum,
		restClient,
	)

	return result, nil
}

// ====================================================================================================
//                                   Private Helper Methods
// ====================================================================================================
func (launcher *TekuCLClientLauncher) getContainerConfigSupplier(
	image string,
	bootnodeContext *cl.CLClientContext, // If this is empty, the node will be launched as a bootnode
	elClientContext *el.ELClientContext,
	logLevel module_io.ParticipantLogLevel,
	validatorKeysDirpathOnModuleContainer string,
	validatorSecretsDirpathOnModuleContainer string,
) func(string, *services.SharedPath) (*services.ContainerConfig, error) {
	containerConfigSupplier := func(privateIpAddr string, sharedDir *services.SharedPath) (*services.ContainerConfig, error) {
		tekuLogLevel, found := tekuLogLevels[logLevel]
		if !found {
			return nil, stacktrace.NewError("No Teku log level defined for client log level '%v'; this is a bug in this module", logLevel)
		}

		genesisConfigYmlSharedPath := sharedDir.GetChildPath(genesisConfigYmlRelFilepathInSharedDir)
		if err := service_launch_utils.CopyFileToSharedPath(launcher.genesisConfigYmlFilepathOnModuleContainer, genesisConfigYmlSharedPath); err != nil {
			return nil, stacktrace.Propagate(
				err,
				"An error occurred copying the genesis config YML from '%v' to shared dir relative path '%v'",
				launcher.genesisConfigYmlFilepathOnModuleContainer,
				genesisConfigYmlRelFilepathInSharedDir,
			)
		}

		genesisSszSharedPath := sharedDir.GetChildPath(genesisSszRelFilepathInSharedDir)
		if err := service_launch_utils.CopyFileToSharedPath(launcher.genesisSszFilepathOnModuleContainer, genesisSszSharedPath); err != nil {
			return nil, stacktrace.Propagate(
				err,
				"An error occurred copying the genesis SSZ from '%v' to shared dir relative path '%v'",
				launcher.genesisSszFilepathOnModuleContainer,
				genesisSszRelFilepathInSharedDir,
			)
		}

		validatorKeysSharedPath := sharedDir.GetChildPath(validatorKeysDirpathRelToSharedDirRoot)
		if err := recursive_copy.Copy(
			validatorKeysDirpathOnModuleContainer,
			validatorKeysSharedPath.GetAbsPathOnThisContainer(),
		); err != nil {
			return nil, stacktrace.Propagate(err, "An error occurred copying the validator keys into the shared directory so the node can consume them")
		}

		validatorSecretsSharedPath := sharedDir.GetChildPath(validatorSecretsDirpathRelToSharedDirRoot)
		if err := recursive_copy.Copy(
			validatorSecretsDirpathOnModuleContainer,
			validatorSecretsSharedPath.GetAbsPathOnThisContainer(),
		); err != nil {
			return nil, stacktrace.Propagate(err, "An error occurred copying the validator secrets into the shared directory so the node can consume them")
		}

		elClientRpcUrlStr := fmt.Sprintf(
			"http://%v:%v",
			elClientContext.GetIPAddress(),
			elClientContext.GetRPCPortNum(),
		)
		cmdArgs := []string{
			"cp",
			"-R",
			validatorKeysSharedPath.GetAbsPathOnServiceContainer(),
			destValidatorKeysDirpathInServiceContainer,
			"&&",
			"cp",
			"-R",
			validatorSecretsSharedPath.GetAbsPathOnServiceContainer(),
			destValidatorSecretsDirpathInServiceContainer,
			"&&",
			tekuBinaryFilepathInImage,
			"--logging=" + tekuLogLevel,
			"--log-destination=CONSOLE",
			"--network=" + genesisConfigYmlSharedPath.GetAbsPathOnServiceContainer(),
			"--initial-state=" + genesisSszSharedPath.GetAbsPathOnServiceContainer(),
			"--data-path=" + consensusDataDirpathOnServiceContainer,
			"--data-storage-mode=PRUNE",
			"--p2p-enabled=true",
			// Set per Pari's recommendation, to reduce noise in the logs
			"--p2p-subscribe-all-subnets-enabled=true",
			fmt.Sprintf("--p2p-peer-lower-bound=%v", minPeers),
			"--eth1-endpoints=" + elClientRpcUrlStr,
			"--Xee-endpoint=" + elClientRpcUrlStr,
			"--p2p-advertised-ip=" + privateIpAddr,
			"--rest-api-enabled=true",
			"--rest-api-docs-enabled=true",
			"--rest-api-interface=0.0.0.0",
			fmt.Sprintf("--rest-api-port=%v", httpPortNum),
			"--rest-api-host-allowlist=*",
			"--data-storage-non-canonical-blocks-enabled=true",
			fmt.Sprintf(
				"--validator-keys=%v:%v",
				destValidatorKeysDirpathInServiceContainer,
				destValidatorSecretsDirpathInServiceContainer,
			),
			"--Xvalidators-suggested-fee-recipient-address=" + validatingRewardsAccount,
		}
		if bootnodeContext != nil {
			cmdArgs = append(cmdArgs, "--p2p-discovery-bootnodes=" + bootnodeContext.GetENR())
		}
		cmdStr := strings.Join(cmdArgs, " ")

		containerConfig := services.NewContainerConfigBuilder(
			image,
		).WithUsedPorts(
			usedPorts,
		).WithEntrypointOverride([]string{
			"sh", "-c",
		}).WithCmdOverride([]string{
			cmdStr,
		}).Build()

		return containerConfig, nil
	}
	return containerConfigSupplier
}
