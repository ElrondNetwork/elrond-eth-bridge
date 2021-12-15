package steps

import (
	"context"

	"github.com/ElrondNetwork/elrond-eth-bridge/core"
	"github.com/ElrondNetwork/elrond-eth-bridge/ethToElrond/v2/elrondToEth"
)

type performTransferStep struct {
	bridge elrondToEth.ElrondToEthBridge
}

func (step *performTransferStep) Execute(ctx context.Context) (core.StepIdentifier, error) {
	wasPerformed, err := step.bridge.WasTransferPerformedOnEthereum(ctx)
	if err != nil {
		step.bridge.GetLogger().Error("error determining if transfer was performed or not", "error", err)
		return elrondToEth.GettingPendingBatchFromElrond, nil
	}

	if wasPerformed {
		step.bridge.GetLogger().Info("transfer performed")
		return elrondToEth.ResolvingSetStatusOnElrond, nil
	}

	if !step.bridge.MyTurnAsLeader() {
		err = step.bridge.PerformTransferOnEthereum(ctx)
		if err != nil {
			step.bridge.GetLogger().Info("error performing action ID", "error", err)
			return elrondToEth.GettingPendingBatchFromElrond, nil
		}
	} else {
		step.bridge.GetLogger().Debug("not my turn as leader in this round")
	}

	return elrondToEth.WaitingTransferConfirmation, nil
}

func (step *performTransferStep) Identifier() core.StepIdentifier {
	return elrondToEth.PerformingTransfer
}

func (step *performTransferStep) IsInterfaceNil() bool {
	return step == nil
}
