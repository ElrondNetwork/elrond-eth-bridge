package ethToElrond

import (
	"context"
	"testing"

	"github.com/ElrondNetwork/elrond-eth-bridge/clients"
	"github.com/ElrondNetwork/elrond-eth-bridge/core"
	"github.com/stretchr/testify/assert"
)

func TestExecuteProposeTransfer(t *testing.T) {
	t.Parallel()

	t.Run("nil batch", func(t *testing.T) {
		t.Parallel()
		bridgeStub := createStubExecutor()
		bridgeStub.GetStoredBatchCalled = func() *clients.TransferBatch {
			return nil
		}

		step := proposeTransferStep{
			bridge: bridgeStub,
		}

		expectedStepIdentifier := core.StepIdentifier(GettingPendingBatchFromEthereum)
		stepIdentifier := step.Execute(context.Background())
		assert.Equal(t, expectedStepIdentifier, stepIdentifier)
	})

	t.Run("error on WasTransferProposedOnElrond", func(t *testing.T) {
		t.Parallel()
		bridgeStub := createStubExecutor()
		bridgeStub.GetStoredBatchCalled = func() *clients.TransferBatch {
			return testBatch
		}
		bridgeStub.WasTransferProposedOnElrondCalled = func(ctx context.Context) (bool, error) {
			return false, expectedError
		}

		step := proposeTransferStep{
			bridge: bridgeStub,
		}

		expectedStepIdentifier := core.StepIdentifier(GettingPendingBatchFromEthereum)
		stepIdentifier := step.Execute(context.Background())
		assert.Equal(t, expectedStepIdentifier, stepIdentifier)
	})

	t.Run("not leader", func(t *testing.T) {
		t.Parallel()
		bridgeStub := createStubExecutor()
		bridgeStub.GetStoredBatchCalled = func() *clients.TransferBatch {
			return testBatch
		}
		bridgeStub.WasTransferProposedOnElrondCalled = func(ctx context.Context) (bool, error) {
			return false, nil
		}
		bridgeStub.MyTurnAsLeaderCalled = func() bool {
			return false
		}

		step := proposeTransferStep{
			bridge: bridgeStub,
		}

		expectedStepIdentifier := step.Identifier()
		stepIdentifier := step.Execute(context.Background())
		assert.Equal(t, expectedStepIdentifier, stepIdentifier)
	})

	t.Run("error on ProposeTransferOnElrond", func(t *testing.T) {
		t.Parallel()
		bridgeStub := createStubExecutor()
		bridgeStub.GetStoredBatchCalled = func() *clients.TransferBatch {
			return testBatch
		}
		bridgeStub.WasTransferProposedOnElrondCalled = func(ctx context.Context) (bool, error) {
			return false, nil
		}
		bridgeStub.MyTurnAsLeaderCalled = func() bool {
			return true
		}
		bridgeStub.ProposeTransferOnElrondCalled = func(ctx context.Context) error {
			return expectedError
		}

		step := proposeTransferStep{
			bridge: bridgeStub,
		}

		expectedStepIdentifier := core.StepIdentifier(GettingPendingBatchFromEthereum)
		stepIdentifier := step.Execute(context.Background())
		assert.Equal(t, expectedStepIdentifier, stepIdentifier)
	})

	t.Run("should work - transfer already proposed", func(t *testing.T) {
		t.Parallel()
		bridgeStub := createStubExecutor()
		bridgeStub.GetStoredBatchCalled = func() *clients.TransferBatch {
			return testBatch
		}
		bridgeStub.WasTransferProposedOnElrondCalled = func(ctx context.Context) (bool, error) {
			return true, nil
		}

		step := proposeTransferStep{
			bridge: bridgeStub,
		}

		expectedStepIdentifier := core.StepIdentifier(SigningProposedTransferOnElrond)
		stepIdentifier := step.Execute(context.Background())
		assert.Equal(t, expectedStepIdentifier, stepIdentifier)
	})

	t.Run("should work", func(t *testing.T) {
		t.Parallel()
		bridgeStub := createStubExecutor()
		bridgeStub.GetStoredBatchCalled = func() *clients.TransferBatch {
			return testBatch
		}
		bridgeStub.WasTransferProposedOnElrondCalled = func(ctx context.Context) (bool, error) {
			return false, nil
		}
		bridgeStub.MyTurnAsLeaderCalled = func() bool {
			return true
		}
		bridgeStub.ProposeTransferOnElrondCalled = func(ctx context.Context) error {
			return nil
		}

		step := proposeTransferStep{
			bridge: bridgeStub,
		}
		// Test IsInterfaceNil
		assert.NotNil(t, step.IsInterfaceNil())

		expectedStepIdentifier := core.StepIdentifier(SigningProposedTransferOnElrond)
		stepIdentifier := step.Execute(context.Background())
		assert.Equal(t, expectedStepIdentifier, stepIdentifier)
	})
}
