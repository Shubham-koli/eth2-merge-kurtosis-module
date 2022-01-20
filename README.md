Ethereum 2 Merge Module
=======================
This is a [Kurtosis module][module-docs] that will:

1. Spin up a network of mining Eth1 clients
1. Spin up a network of Eth2 Beacon/validator clients
1. Add [a transaction spammer](https://github.com/kurtosis-tech/tx-fuzz) that will repeatedly send transactions to the network
1. Launch [a consensus monitor](https://github.com/ralexstokes/ethereum_consensus_monitor) instance attached to the network
1. Perform the merge
1. Optionally block until the Beacon nodes finalize an epoch (i.e. finalized_epoch > 0 and finalized_epoch = current_epoch - 3)

### Quickstart
1. [Install Docker if you haven't done so already][docker-installation]
1. [Install the Kurtosis CLI, or upgrade it to the latest version if it's already installed][kurtosis-cli-installation]
1. Ensure your Docker engine is running:
    ```
    docker image ls
    ```
1. Execute the module:
    ```
    kurtosis module exec --enclave-id eth2 kurtosistech/eth2-merge-kurtosis-module --execute-params '{}'
    ```

To configure the module behaviour, provide a non-empty JSON object to the `--execute-params` flag. The JSON schema that can be passed in is as follows with the defaults provided (though note that the `//` comments are for explanation purposes and aren't valid JSON so need to be removed):

```javascript
{
    // Specification of the participants in the network
    "participants": [
        {
            // The type of EL client that should be started
            // Valid values are "geth" and "nethermind"
            "elType": "geth",

            // The Docker image that should be used for the EL client; leave blank to use the default for the client type
            // Defaults by client:
            // - geth: parithoshj/geth:merge-f72c361"
            // - nethermind: nethermindeth/nethermind:kintsugi_0.5
            "elImage": "",

            // The type of CL client that should be started
            // Valid values are "nimbus", "lighthouse", "lodestar", "teku", and "prysm"
            "clType": "nimbus",

            // The Docker image that should be used for the EL client; leave blank to use the default for the client type
            // Defaults by client (note that Prysm is different in that it requires two images - a Beacon and a validator - separated by a comma):
            // - lighthouse: sigp/lighthouse:latest-unstable",
            // - teku: consensys/teku:latest",
            // - nimbus: statusim/nimbus-eth2:amd64-latest",
            // - prysm: prysmaticlabs/prysm-beacon-chain:latest,prysmaticlabs/prysm-validator:latest",
            // - lodestar: chainsafe/lodestar:next",
            "clImage": ""
        }
    ],

    // Configuration parameters for the Eth network
    "network": {
	// The network ID of the Eth1 network
	"networkId": "3151908",

	// The address of the staking contract address on the Eth1 chain
	"depositContractAddress": "0x4242424242424242424242424242424242424242",

	// Number of seconds per slot on the Beacon chain
	"secondsPerSlot": 12,

	// Number of slots in an epoch on the Beacon chain
	"slotsPerEpoch": 32,

	// Must come before the merge fork epoch
	// See https://notes.ethereum.org/@ExXcnR0-SJGthjz1dwkA1A/H1MSKgm3F
	"altairForkEpoch": 1,

	// Must occur before the total terminal difficulty is hit on the Eth1 chain
	// See https://notes.ethereum.org/@ExXcnR0-SJGthjz1dwkA1A/H1MSKgm3F
	"mergeForkEpoch": 2,

	// Once the total difficulty of all mined blocks crosses this threshold, the Eth1 chain will
	//  merge with the Beacon chain
	// Must happen after the merge fork epoch on the Beacon chain
	// See https://notes.ethereum.org/@ExXcnR0-SJGthjz1dwkA1A/H1MSKgm3F
	"totalTerminalDifficulty": 100000000,

	// The number of validator keys that each CL validator node should get
	"numValidatorKeysPerNode": 64,

	// This mnemonic will a) be used to create keystores for all the types of validators that we have and b) be used to generate a CL genesis.ssz that has the children
	//  validator keys already preregistered as validators
	"preregisteredValidatorKeysMnemonic": "giant issue aisle success illegal bike spike question tent bar rely arctic volcano long crawl hungry vocal artwork sniff fantasy very lucky have athlete"
    },

    // If set to false, we won't wait for the EL clients to mine at least 1 block before proceeding with adding the CL clients
    // This is purely for debug purposes; waiting for blockNumber > 0 is required for the CL network to behave as
    //  expected, but that wait can be several minutes. Skipping the wait can be a good way to shorten the debug loop on a
    //  CL client that's failing to start.
    "waitForMining": true,

    // If set, the module will block until a finalized epoch has occurred
    "waitForFinalization": false,

    // The log level that the clients should log at
    // Valid values are "error", "warn", "info", "debug", and "trace"
    "logLevel": "info"
}
```



defined in Go [here](https://github.com/kurtosis-tech/eth2-merge-kurtosis-module/blob/develop/kurtosis-module/impl/module_io/params.go#L46) (look for the `json:"XXXXXX"` tags of the object to determine the JSON field names), and the default values that will be used if you omit any fields are defined [here](https://github.com/kurtosis-tech/eth2-merge-kurtosis-module/blob/develop/kurtosis-module/impl/module_io/default_params.go#L4).

### Management
Kurtosis will create a new enclave to house the services of the Ethereum network. [This page][using-the-cli] contains documentation for managing the created enclave & viewing detailed information about it.

<!-- Only links below here -->
[docker-installation]: https://docs.docker.com/get-docker/
[kurtosis-cli-installation]: https://docs.kurtosistech.com/installation.html
[module-docs]: https://docs.kurtosistech.com/modules.html
[using-the-cli]: https://docs.kurtosistech.com/using-the-cli.html

### Development
The unit tests in this module also require Kurtosis to be available.
