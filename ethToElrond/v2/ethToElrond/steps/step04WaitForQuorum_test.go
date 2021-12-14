package steps

import (
	"context"
	"testing"

	"github.com/ElrondNetwork/elrond-eth-bridge/core"
	"github.com/ElrondNetwork/elrond-eth-bridge/ethToElrond/v2/ethToElrond"
	"github.com/stretchr/testify/assert"
)

func TestExecuteWaitForQuorumStep(t *testing.T) {
	t.Parallel()

	t.Run("error on IsQuorumReached", func(t *testing.T) {
		bridgeStub := createStubExecutor()
		bridgeStub.IsQuorumReachedOnElrondCalled = func(ctx context.Context) (bool, error) {
			return false, expectedError
		}

		step := waitForQuorumStep{
			bridge: bridgeStub,
		}

		expectedStepIdentifier := core.StepIdentifier(ethToElrond.GettingPendingBatchFromEthereum)
		stepIdentifier, err := step.Execute(context.Background())
		assert.Nil(t, err)
		assert.Equal(t, expectedStepIdentifier, stepIdentifier)
	})

	t.Run("should work - quorum not reached", func(t *testing.T) {
		bridgeStub := createStubExecutor()
		bridgeStub.IsQuorumReachedOnElrondCalled = func(ctx context.Context) (bool, error) {
			return false, nil
		}

		step := waitForQuorumStep{
			bridge: bridgeStub,
		}

		expectedStepIdentifier := core.StepIdentifier(ethToElrond.WaitingForQuorum)
		stepIdentifier, err := step.Execute(context.Background())
		assert.Nil(t, err)
		assert.Equal(t, expectedStepIdentifier, stepIdentifier)
	})

	t.Run("should work", func(t *testing.T) {
		bridgeStub := createStubExecutor()
		bridgeStub.IsQuorumReachedOnElrondCalled = func(ctx context.Context) (bool, error) {
			return true, nil
		}

		step := waitForQuorumStep{
			bridge: bridgeStub,
		}
		// Test Identifier()
		expectedStepIdentifier := core.StepIdentifier(ethToElrond.WaitingForQuorum)
		assert.Equal(t, expectedStepIdentifier, step.Identifier())
		// Test IsInterfaceNil
		assert.NotNil(t, step.IsInterfaceNil())

		expectedStepIdentifier = ethToElrond.PerformingActionID
		stepIdentifier, err := step.Execute(context.Background())
		assert.Nil(t, err)
		assert.Equal(t, expectedStepIdentifier, stepIdentifier)
	})

	t.Run("max retries reached", func(t *testing.T) {
		bridgeStub := createStubExecutor()
		bridgeStub.ProcessMaxRetriesOnElrondCalled = func() bool {
			return true
		}

		step := waitForQuorumStep{
			bridge: bridgeStub,
		}

		expectedStepIdentifier := core.StepIdentifier(ethToElrond.GettingPendingBatchFromEthereum)
		stepIdentifier, err := step.Execute(context.Background())
		assert.Nil(t, err)
		assert.Equal(t, expectedStepIdentifier, stepIdentifier)
	})
}