package impl

import (
	"encoding/json"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/forkmon"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/cl"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/cl/cl_client_rest_client"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/cl/lighthouse"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/cl/lodestar"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/cl/nimbus"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/cl/prysm"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/cl/teku"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/el"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/el/geth"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/participant_network/el/nethermind"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/prelaunch_data_generator"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/prelaunch_data_generator/genesis_consts"
	"github.com/kurtosis-tech/eth2-merge-kurtosis-module/kurtosis-module/impl/transaction_spammer"
	"github.com/kurtosis-tech/kurtosis-core-api-lib/api/golang/lib/enclaves"
	"github.com/kurtosis-tech/stacktrace"
	"github.com/sirupsen/logrus"
	"path"
	"text/template"
	"time"
)

const (
	networkId = "3151908"

	// The number of validator keys that will be preregistered inside the CL genesis file when it's created
	numValidatorsToPreregister = 100

	// ----------------------------------- Params Constants -----------------------------------------
	gethClientKeyword = "geth"
	nethermindClientKeyword = "nethermind"
	nimbusClientKeyword = "nimbus"
	tekuClientKeyword = "teku"
	lodestarClientKeyword = "lodestar"
	lighthouseClientKeyword = "lighthouse"
	prysmClientKeyword = "prysm"

	defaultWaitForFinalization = false
	// --------------------------------- End Params Constants ---------------------------------------

	// ----------------------------------- Genesis Config Constants -----------------------------------------
	// Seems to be hardcoded
	slotsPerEpoch = uint32(32)

	// If we drop this, things start to behave strangely, with slots that are of variable time lengths
	secondsPerSlot = uint32(12)

	altairForkEpoch = uint64(1)  // Set per Parithosh's recommendation
	mergeForkEpoch = uint64(2)   // Set per Parithosh's recommendation
	// TODO Should be set to roughly one hour (??) so that this is reached AFTER the CL gets the merge fork version (per Parithosh)
	totalTerminalDifficulty  = uint64(60000000)

	// This is the mnemonic that will be used to generate validator keys which will be preregistered in the CL genesis.ssz that we create
	// This is the same mnemonic that should be used to generate the validator keys that we'll load into our CL nodes when we run them
	preregisteredValidatorKeysMnemonic = "giant issue aisle success illegal bike spike question tent bar rely arctic volcano long crawl hungry vocal artwork sniff fantasy very lucky have athlete"

	// TODO What units are these?
	genesisDelay = 0
	// --------------------------------- End Genesis Config Constants ----------------------------------------

	// ----------------------------------- Static File Constants -----------------------------------------
	staticFilesDirpath                    = "/static-files"

	// Geth + CL genesis generation
	genesisGenerationConfigDirpath = staticFilesDirpath + "/genesis-generation-config"
	gethGenesisGenerationConfigYmlTemplateFilepath = genesisGenerationConfigDirpath + "/el/genesis-config.yaml.tmpl"
	clGenesisGenerationConfigYmlTemplateFilepath = genesisGenerationConfigDirpath + "/cl/config.yaml.tmpl"
	clGenesisGenerationMnemonicsYmlTemplateFilepath = genesisGenerationConfigDirpath + "/cl/mnemonics.yaml.tmpl"

	// Nethermind
	nethermindGenesisJsonTemplateFilepath = staticFilesDirpath + "/nethermind-genesis.json.tmpl"

	// Prysm
	prysmPasswordTxtTemplateFilepath = staticFilesDirpath + "/prysm-password.txt.tmpl"

	// Forkmon config
	forkmonConfigTemplateFilepath = staticFilesDirpath + "/forkmon-config/config.toml.tmpl"
	// --------------------------------- End Static File Constants ----------------------------------------

	responseJsonLinePrefixStr = ""
	responseJsonLineIndentStr = "  "

	// TODO uncomment these when the module can either start a private network OR connect to an existing devnet
	// mergeDevnet3NetworkId = "1337602"
	// mergeDevnet3ClClientBootnodeEnr = "enr:-Iq4QKuNB_wHmWon7hv5HntHiSsyE1a6cUTK1aT7xDSU_hNTLW3R4mowUboCsqYoh1kN9v3ZoSu_WuvW9Aw0tQ0Dxv6GAXxQ7Nv5gmlkgnY0gmlwhLKAlv6Jc2VjcDI1NmsxoQK6S-Cii_KmfFdUJL2TANL3ksaKUnNXvTCv1tLwXs0QgIN1ZHCCIyk"

	// In normal operation, the finalized epoch will be this many epochs behind head
	expectedNumEpochsBehindHeadForFinalizedEpoch = uint64(3)
	firstHeadEpochWhereFinalizedEpochIsPossible = expectedNumEpochsBehindHeadForFinalizedEpoch + 1
	timeBetweenFinalizedEpochChecks = 5 * time.Second
	// TODO FIGURE OUT WHY THIS HAPPENS AND GET RID OF IT
	extraDelayBeforeSlotCountStartsIncreasing = 4 * time.Minute
)
/*
var mergeDevnet3BootnodeEnodes = []string{
	"enode://6b457d42e6301acfae11dc785b43346e195ad0974b394922b842adea5aeb4c55b02410607ba21e4a03ba53e7656091e2f990034ce3f8bad4d0cca1c6398bdbb8@137.184.55.117:30303",
	"enode://588ef56694223ce3212d7c56e5b6f3e8ba46a9c29522fdc6fef15657f505a7314b9bd32f2d53c4564bc6b9259c3d5c79fc96257eff9cd489004c4d9cbb3c0707@137.184.203.157:30303",
	"enode://46b2ecd18c24463413b7328e9a59c72d955874ad5ddb9cd9659d322bedd2758a6cefb8378e2309a028bd3cdf2beca0b18c3457f03e772f35d0cd06c37ce75eee@137.184.213.208:30303",
}
 */
// Defines the strings users can use to define the types of EL clients the participant network will contain
var elClientKeywords = map[string]participant_network.ParticipantELClientType{
	gethClientKeyword: participant_network.ParticipantELClientType_Geth,
	nethermindClientKeyword: participant_network.ParticipantELClientType_Nethermind,
}
// Defines the strings users can use to define the types of CL clients the participant network will contain
var clClientKeywords = map[string]participant_network.ParticipantCLClientType{
	nimbusClientKeyword: participant_network.ParticipantCLClientType_Nimbus,
	lighthouseClientKeyword: participant_network.ParticipantCLClientType_Lighthouse,
	lodestarClientKeyword: participant_network.ParticipantCLClientType_Lodestar,
	prysmClientKeyword: participant_network.ParticipantCLClientType_Prysm,
	tekuClientKeyword: participant_network.ParticipantCLClientType_Teku,
}
var defaultParticipants = []*ParticipantParams{
	{
		ELClientKeyword: gethClientKeyword,
		CLClientKeyword: nimbusClientKeyword,
	},
}

type ParticipantParams struct {
	ELClientKeyword string `json:"el"`
	CLClientKeyword string `json:"cl"`
}
type ExecuteParams struct {
	// Participants
	Participants []*ParticipantParams	`json:"participants"`

	WaitForFinalization bool	`json:"waitForFinalization"`
}

type ExecuteResponse struct {
	ForkmonPublicURL string	`json:"forkmonUrl"`
}

type Eth2KurtosisModule struct {
}

func NewEth2KurtosisModule() *Eth2KurtosisModule {
	return &Eth2KurtosisModule{}
}

func (e Eth2KurtosisModule) Execute(enclaveCtx *enclaves.EnclaveContext, serializedParams string) (serializedResult string, resultError error) {
	logrus.Info("Deserializing execute params...")
	paramsObj, err := deserializeAndValidateParams(serializedParams)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred deserializing & validating the params")
	}
	numParticipants := len(paramsObj.Participants)
	logrus.Info("Successfully deserialized execute params")

	logrus.Info("Generating prelaunch data...")
	genesisUnixTimestamp := time.Now().Unix()
	gethGenesisConfigTemplate, err := parseTemplate(gethGenesisGenerationConfigYmlTemplateFilepath)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred parsing the Geth genesis generation config YAML template")
	}
	clGenesisConfigTemplate, err := parseTemplate(clGenesisGenerationConfigYmlTemplateFilepath)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred parsing the CL genesis generation config YAML template")
	}
	clGenesisMnemonicsYmlTemplate, err := parseTemplate(clGenesisGenerationMnemonicsYmlTemplateFilepath)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred parsing the CL mnemonics YAML template")
	}
	nethermindGenesisJsonTemplate, err := parseTemplate(nethermindGenesisJsonTemplateFilepath)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred parsing the Nethermind genesis json template")
	}
	prysmPassowordTxtTemplate, err := parseTemplate(prysmPasswordTxtTemplateFilepath)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred parsing the Prysm password txt template")
	}
	prelaunchData, err := prelaunch_data_generator.GeneratePrelaunchData(
		enclaveCtx,
		gethGenesisConfigTemplate,
		clGenesisConfigTemplate,
		clGenesisMnemonicsYmlTemplate,
		preregisteredValidatorKeysMnemonic,
		numValidatorsToPreregister,
		uint32(numParticipants),
		genesisUnixTimestamp,
		genesisDelay,
		networkId,
		secondsPerSlot,
		altairForkEpoch,
		mergeForkEpoch,
		totalTerminalDifficulty,
		preregisteredValidatorKeysMnemonic,
	)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred launching the Ethereum genesis generator Service")
	}
	logrus.Info("Successfully generated prelaunch data")

	logrus.Info("Creating EL & CL client launchers...")
	elClientLaunchers := map[participant_network.ParticipantELClientType]el.ELClientLauncher{
		participant_network.ParticipantELClientType_Geth: geth.NewGethELClientLauncher(
			prelaunchData.GethELGenesisJsonFilepathOnModuleContainer,
			genesis_consts.PrefundedAccounts,
		),
		participant_network.ParticipantELClientType_Nethermind: nethermind.NewNethermindELClientLauncher(
			nethermindGenesisJsonTemplate,
			totalTerminalDifficulty,
		),
	}
	clGenesisPaths := prelaunchData.CLGenesisPaths
	clClientLaunchers := map[participant_network.ParticipantCLClientType]cl.CLClientLauncher{
		participant_network.ParticipantCLClientType_Teku: teku.NewTekuCLClientLauncher(
			clGenesisPaths.GetConfigYMLFilepath(),
			clGenesisPaths.GetGenesisSSZFilepath(),
		),
		participant_network.ParticipantCLClientType_Nimbus: nimbus.NewNimbusLauncher(
			clGenesisPaths.GetParentDirpath(),
		),
		participant_network.ParticipantCLClientType_Lodestar: lodestar.NewLodestarCLClientLauncher(
			clGenesisPaths.GetConfigYMLFilepath(),
			clGenesisPaths.GetGenesisSSZFilepath(),
	    ),
		participant_network.ParticipantCLClientType_Lighthouse: lighthouse.NewLighthouseCLClientLauncher(
			clGenesisPaths.GetParentDirpath(),
		 ),
		participant_network.ParticipantCLClientType_Prysm: prysm.NewPrysmCLCLientLauncher(
			clGenesisPaths.GetConfigYMLFilepath(),
			clGenesisPaths.GetGenesisSSZFilepath(),
			prelaunchData.KeystoresGenerationResult.PrysmPassword,
			prysmPassowordTxtTemplate,
		),
	}
	logrus.Info("Successfully created EL & CL client launchers")

	logrus.Infof("Adding %v participants...", numParticipants)
	keystoresGenerationResult := prelaunchData.KeystoresGenerationResult
	network := participant_network.NewParticipantNetwork(
		enclaveCtx,
		networkId,
		keystoresGenerationResult.PerNodeKeystoreDirpaths,
		elClientLaunchers,
		clClientLaunchers,
	)

	allElClientContexts := []*el.ELClientContext{}
	allClClientContexts := []*cl.CLClientContext{}
	for idx, participantParams := range paramsObj.Participants {
		elClientTypeKeyword := participantParams.ELClientKeyword
		clClientTypeKeyword := participantParams.CLClientKeyword

		// Don't need to validate because we already did when deserializing
		elClientType := elClientKeywords[elClientTypeKeyword]
		clClientType := clClientKeywords[clClientTypeKeyword]
		participant, err := network.AddParticipant(
			elClientType,
			clClientType,
		)
		if err != nil {
			return "", stacktrace.Propagate(
				err,
				"An error occurred adding participant %v with EL type '%v' and CL type '%v'",
				idx,
				elClientTypeKeyword,
				clClientTypeKeyword,
			 )
		}
		allElClientContexts = append(allElClientContexts, participant.GetELClientContext())
		allClClientContexts = append(allClClientContexts, participant.GetCLClientContext())
	}
	logrus.Infof("Successfully added %v partitipcants", numParticipants)


	logrus.Info("Launching transaction spammer...")
	// TODO Upgrade the transaction spammer so it can take in multiple EL client addresses
	if err := transaction_spammer.LaunchTransanctionSpammer(enclaveCtx, genesis_consts.PrefundedAccounts, allElClientContexts[0]); err != nil {
		return "", stacktrace.Propagate(err, "An error occurred launching the transaction spammer")
	}
	logrus.Info("Successfully launched transaction spammer")

	logrus.Info("Launching forkmon...")
	forkmonConfigTemplate, err := parseTemplate(forkmonConfigTemplateFilepath)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred parsing forkmon config template file '%v'", forkmonConfigTemplateFilepath)
	}
	forkmonPublicUrl, err := forkmon.LaunchForkmon(
		enclaveCtx,
		forkmonConfigTemplate,
		allClClientContexts,
		genesisUnixTimestamp,
		secondsPerSlot,
	)
	logrus.Info("Successfully launched forkmon")

	if paramsObj.WaitForFinalization {
		logrus.Info("Waiting for the first finalized epoch...")
		firstClClientCtx := allClClientContexts[0]
		firstClClientRestClient := firstClClientCtx.GetRESTClient()
		if err := waitUntilFirstFinalizedEpoch(firstClClientRestClient); err != nil {
			return "", stacktrace.Propagate(err, "An error occurred waiting until the first finalized epoch occurred")
		}
		logrus.Info("First finalized epoch occurred successfully")
	}

	responseObj := &ExecuteResponse{
		ForkmonPublicURL: forkmonPublicUrl,
	}
	responseStr, err := json.MarshalIndent(responseObj, responseJsonLinePrefixStr, responseJsonLineIndentStr)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred serializing the following response object to JSON for returning: %+v", responseObj)
	}

	return string(responseStr), nil
}

func deserializeAndValidateParams(paramsStr string) (*ExecuteParams, error) {
	paramsObj := &ExecuteParams{
		Participants: defaultParticipants,
		WaitForFinalization: defaultWaitForFinalization,
	}
	if err := json.Unmarshal([]byte(paramsStr), paramsObj); err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred deserializing the serialized params")
	}
	if len(paramsObj.Participants) == 0 {
		return nil, stacktrace.NewError("At least one participant is required")
	}
	for idx, participant := range paramsObj.Participants {
		if idx == 0 && participant.ELClientKeyword == nethermindClientKeyword {
			return nil, stacktrace.NewError("Cannot use a Nethermind client for the first participant because Nethermind clients don't mine on Eth1")
		}

		elClientKeyword := participant.ELClientKeyword
		if _, found := elClientKeywords[elClientKeyword]; !found {
			return nil, stacktrace.NewError("Participant %v declares unrecognized EL client type '%v'", idx, elClientKeyword)
		}

		clClientKeyword := participant.CLClientKeyword
		if _, found := clClientKeywords[clClientKeyword]; !found {
			return nil, stacktrace.NewError("Participant %v declares unrecognized CL client type '%v'", idx, clClientKeyword)
		}
	}
	return paramsObj, nil
}

func parseTemplate(filepath string) (*template.Template, error) {
	tmpl, err := template.New(
		// For some reason, the template name has to match the basename of the file:
		//  https://stackoverflow.com/questions/49043292/error-template-is-an-incomplete-or-empty-template
		path.Base(filepath),
	).ParseFiles(
		filepath,
	)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred parsing template file '%v'", filepath)
	}
	return tmpl, nil
}

func waitUntilFirstFinalizedEpoch(restClient *cl_client_rest_client.CLClientRESTClient) error {
	// If we wait long enough that we might be in this epoch, we've waited too long - finality should already have happened
	waitedTooLongEpoch := firstHeadEpochWhereFinalizedEpochIsPossible + 1
	timeoutSeconds := waitedTooLongEpoch * uint64(slotsPerEpoch) * uint64(secondsPerSlot)
	timeout := time.Duration(timeoutSeconds) * time.Second + extraDelayBeforeSlotCountStartsIncreasing
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		currentSlot, err := restClient.GetCurrentSlot()
		if err != nil {
			return stacktrace.Propagate(err, "An error occurred getting the current slot using the REST client, which should never happen")
		}
		currentEpoch := currentSlot / uint64(slotsPerEpoch)
		finalizedEpoch, err := restClient.GetFinalizedEpoch()
		if err != nil {
			return stacktrace.Propagate(err, "An error occurred getting the finalized epoch using the REST client, which should never happen")
		}
		if finalizedEpoch > 0 && finalizedEpoch + expectedNumEpochsBehindHeadForFinalizedEpoch == currentEpoch {
			return nil
		}
		logrus.Debugf(
			"Finalized epoch hasn't occurred yet; current slot = '%v', current epoch = '%v', and finalized epoch = '%v'",
			currentSlot,
			currentEpoch,
			finalizedEpoch,
		 )
		time.Sleep(timeBetweenFinalizedEpochChecks)
	}
	return stacktrace.NewError("Waited for %v for the finalized epoch to be %v epochs behind the current epoch, but it didn't happen", timeout, expectedNumEpochsBehindHeadForFinalizedEpoch)
}
