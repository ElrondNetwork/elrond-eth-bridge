package relayers

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ElrondNetwork/elrond-eth-bridge/bridge"
	"github.com/ElrondNetwork/elrond-eth-bridge/bridge/eth/contract"
	"github.com/ElrondNetwork/elrond-eth-bridge/core"
	"github.com/ElrondNetwork/elrond-eth-bridge/integrationTests"
	"github.com/ElrondNetwork/elrond-eth-bridge/integrationTests/mock"
	"github.com/ElrondNetwork/elrond-eth-bridge/relay"
	"github.com/ElrondNetwork/elrond-go/config"
	"github.com/ElrondNetwork/elrond-go/p2p"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayersShouldExecuteTransferFromEthToElrond(t *testing.T) {
	if testing.Short() {
		t.Skip("this is not a short test")
	}

	token1Erc20 := integrationTests.CreateRandomEthereumAddress()
	ticker1 := "tck-000001"

	token2Erc20 := integrationTests.CreateRandomEthereumAddress()
	ticker2 := "tck-000002"

	value1 := big.NewInt(111111111)
	destination1 := integrationTests.CreateRandomElrondAddress()

	value2 := big.NewInt(222222222)
	destination2 := integrationTests.CreateRandomElrondAddress()

	batch := contract.Batch{
		Nonce:                  big.NewInt(1),
		Timestamp:              big.NewInt(0),
		LastUpdatedBlockNumber: big.NewInt(0),
		Deposits: []contract.Deposit{
			{
				Nonce:        big.NewInt(0),
				TokenAddress: token1Erc20,
				Amount:       value1,
				Depositor:    common.Address{},
				Recipient:    destination1.AddressBytes(),
				Status:       0,
			},
			{
				Nonce:        big.NewInt(0),
				TokenAddress: token2Erc20,
				Amount:       value2,
				Depositor:    common.Address{},
				Recipient:    destination2.AddressBytes(),
				Status:       0,
			},
		},
		Status: 0,
	}

	ethereumChainMock := mock.NewEthereumChainMock()
	ethereumChainMock.SetPendingBatch(batch)
	ethereumChainMock.SetQuorum(3)

	elrondChainMock := mock.NewElrondChainMock()
	elrondChainMock.AddTokensPair(token1Erc20, ticker1)
	elrondChainMock.AddTokensPair(token2Erc20, ticker2)
	elrondChainMock.GetStatusesAfterExecutionHandler = func() []byte {
		return []byte{bridge.Executed, bridge.Rejected}
	}

	numRelayers := 3
	relayers := make([]*relay.Relay, 0, numRelayers)
	defer func() {
		for _, r := range relayers {
			_ = r.Stop()
		}
	}()

	messengers := integrationTests.CreateLinkedMessengers(numRelayers)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()
	ethereumChainMock.ProcessFinishedHandler = func() {
		time.Sleep(time.Second * 20)

		cancel()
	}

	for i := 0; i < numRelayers; i++ {
		argsRelay := createMockRelayArgs(i, messengers[i], elrondChainMock, ethereumChainMock)
		r, err := relay.NewRelay(argsRelay)
		require.Nil(t, err)

		elrondChainMock.AddRelayer(r.ElrondAddress())
		ethereumChainMock.AddRelayer(r.EthereumAddress())

		go func() {
			err = r.Start(ctx)
			integrationTests.Log.LogIfError(err)
			require.Nil(t, err)
		}()

		relayers = append(relayers, r)
	}

	<-ctx.Done()

	setStatus := ethereumChainMock.GetLastProposedStatus()
	require.NotNil(t, setStatus)
	assert.Equal(t, 3, len(setStatus.Signatures))
	assert.Equal(t, []byte{bridge.Executed, bridge.Rejected}, setStatus.NewDepositStatuses)

	assert.NotNil(t, elrondChainMock.PerformedActionID())
	transfer := elrondChainMock.ProposedTransfer()
	require.NotNil(t, transfer)
	require.Equal(t, 2, len(transfer.Transfers))

	assert.Equal(t, destination1.AddressBytes(), transfer.Transfers[0].To)
	assert.Equal(t, hex.EncodeToString([]byte(ticker1)), transfer.Transfers[0].Token)
	assert.Equal(t, value1, transfer.Transfers[0].Amount)

	assert.Equal(t, destination2.AddressBytes(), transfer.Transfers[1].To)
	assert.Equal(t, hex.EncodeToString([]byte(ticker2)), transfer.Transfers[1].Token)
	assert.Equal(t, value2, transfer.Transfers[1].Amount)
}

func createMockRelayArgs(
	index int,
	messenger p2p.Messenger,
	elrondChainMock *mock.ElrondChainMock,
	ethereumChainMock *mock.EthereumChainMock,
) relay.ArgsRelayer {

	return relay.ArgsRelayer{
		Config: createMockRelayConfig(index),
		FlagsConfig: relay.ContextFlagsConfig{
			RestApiInterface: core.WebServerOffString,
		},
		Name:        "eth <-> elrond",
		Proxy:       elrondChainMock,
		EthClient:   ethereumChainMock,
		EthInstance: ethereumChainMock,
		Messenger:   messenger,
	}
}

func createMockRelayConfig(index int) relay.Config {
	stateMachineConfig := relay.ConfigStateMachine{
		StepDurationInMillis: 1000,
		Steps: []relay.StepConfig{
			{Name: "getting the pending transactions", DurationInMillis: 1000},
			{Name: "proposing transfer", DurationInMillis: 1000},
			{Name: "waiting signatures for propose transfer", DurationInMillis: 1000},
			{Name: "executing transfer", DurationInMillis: 1000},
			{Name: "proposing set status", DurationInMillis: 1000},
			{Name: "waiting signatures for propose set status", DurationInMillis: 1000},
			{Name: "executing set status", DurationInMillis: 1000},
		},
	}

	return relay.Config{
		Eth: bridge.EthereumConfig{
			NetworkAddress:               "mock",
			BridgeAddress:                "3009d97FfeD62E57d444e552A9eDF9Ee6Bc8644c",
			PrivateKeyFile:               fmt.Sprintf("testdata/ethereum%d.sk", index),
			IntervalToResendTxsInSeconds: 10,
			GasLimit:                     500000,
			GasStation: bridge.GasStationConfig{
				Enabled: false,
			},
		},
		Elrond: bridge.ElrondConfig{
			NetworkAddress:               "mock",
			BridgeAddress:                "erd1qqqqqqqqqqqqqpgqzyuaqg3dl7rqlkudrsnm5ek0j3a97qevd8sszj0glf",
			PrivateKeyFile:               fmt.Sprintf("testdata/elrond%d.pem", index),
			IntervalToResendTxsInSeconds: 10,
		},
		P2P: relay.ConfigP2P{},
		StateMachine: map[string]relay.ConfigStateMachine{
			"EthToElrond": stateMachineConfig,
			"ElrondToEth": stateMachineConfig,
		},
		Relayer: relay.ConfigRelayer{
			Marshalizer: config.MarshalizerConfig{
				Type:           "json",
				SizeCheckDelta: 10,
			},
			RoleProvider: relay.RoleProviderConfig{
				PollingIntervalInMillis: 1000,
			},
		},
	}
}