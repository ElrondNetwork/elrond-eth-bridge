package v2

import (
    "context"
    "fmt"

    "github.com/ElrondNetwork/elrond-eth-bridge/clients"
    "github.com/ElrondNetwork/elrond-go-core/core/check"
    logger "github.com/ElrondNetwork/elrond-go-logger"
    "github.com/ethereum/go-ethereum/common"
)

// ArgsBaseBridge is the common arguments DTO struct used in both bridges
type ArgsBaseBridgeExecutor struct {
    Log                      logger.Logger
    ElrondClient             ElrondClient
    EthereumClient           EthereumClient
}

// ArgsEthToElrondBridgeExectutor is the arguments DTO struct used in the for the Eth->Elrond bridge
type ArgsEthToElrondBridgeExectutor struct {
    ArgsBaseBridgeExecutor
    TopologyProviderOnElrond TopologyProvider
}

// ArgsElrondToEthBridgeExectutor is the arguments DTO struct used in the for the Elrond->Eth bridge
type ArgsElrondToEthBridgeExectutor struct {
    ArgsBaseBridgeExecutor
    TopologyProviderOnElrond   TopologyProvider
    TopologyProviderOnEthereum TopologyProvider
}

type bridgeExecutor struct {
    log                        logger.Logger
    topologyProviderOnElrond   TopologyProvider
    topologyProviderOnEthereum TopologyProvider
    elrondClient               ElrondClient
    ethereumClient             EthereumClient
    batch                      *clients.TransferBatch
    actionID                   uint64
    msgHash                    common.Hash
    retriesOnElrond            uint64
    retriesOnEthereum          uint64
}

// CreateEthToElrondBridgeExecutor will create an Eth->Elrond bridge executor
func CreateEthToElrondBridgeExecutor(args ArgsEthToElrondBridgeExectutor) (*bridgeExecutor, error) {
    err := checkBaseArgs(args.ArgsBaseBridgeExecutor)
    if err != nil {
        return nil, err
    }
    if check.IfNil(args.TopologyProviderOnElrond) {
        return nil, ErrNilElrondTopologyProvider
    }

    executor := createBaseBridgeExecutor(args.ArgsBaseBridgeExecutor)
    executor.topologyProviderOnElrond = args.TopologyProviderOnElrond
    return executor, nil
}

// CreateElrondToEthBridgeExecutor will create an Elrond->Eth bridge executor
func CreateElrondToEthBridgeExecutor(args ArgsElrondToEthBridgeExectutor) (*bridgeExecutor, error) {
    err := checkBaseArgs(args.ArgsBaseBridgeExecutor)
    if err != nil {
        return nil, err
    }
    if check.IfNil(args.TopologyProviderOnElrond) {
        return nil, ErrNilElrondTopologyProvider
    }
    if check.IfNil(args.TopologyProviderOnEthereum) {
        return nil, ErrNilEthereumTopologyProvider
    }

    executor := createBaseBridgeExecutor(args.ArgsBaseBridgeExecutor)
    executor.topologyProviderOnElrond = args.TopologyProviderOnElrond
    executor.topologyProviderOnEthereum = args.TopologyProviderOnEthereum
    return executor, nil
}

func checkBaseArgs (args ArgsBaseBridgeExecutor) error {
    if check.IfNil(args.Log) {
        return ErrNilLogger
    }
    if check.IfNil(args.ElrondClient) {
        return ErrNilElrondClient
    }
    if check.IfNil(args.EthereumClient) {
        return ErrNilEthereumClient
    }
    return nil
}

func createBaseBridgeExecutor(args ArgsBaseBridgeExecutor) *bridgeExecutor {
    return &bridgeExecutor{
        log:                      args.Log,
        elrondClient:             args.ElrondClient,
        ethereumClient:           args.EthereumClient,
    }
}

// GetLogger returns the logger implementation
func (executor *bridgeExecutor) GetLogger() logger.Logger {
    return executor.log
}

// MyTurnAsLeaderOnElrond returns true if the current relayer node is the leader on Elrond
func (executor *bridgeExecutor) MyTurnAsLeaderOnElrond() bool {
    return executor.topologyProviderOnElrond.MyTurnAsLeader()
}

// GetAndStoreBatchFromElrond fetches the pending batch from Elrond and stores it
func (executor *bridgeExecutor) GetAndStoreBatchFromElrond(ctx context.Context) error {
    batch, err := executor.elrondClient.GetPending(ctx)
    if err != nil {
        return err
    }

    executor.batch = batch
    executor.log.Info("got pending batch from Elrond", "batch ID", executor.batch.ID)

    return nil
}

// GetStoredBatch returns the stored batch
func (executor *bridgeExecutor) GetStoredBatch() *clients.TransferBatch {
    return executor.batch
}

// GetLastExecutedEthBatchIDFromElrond returns the last executed batch ID that is stored on the Elrond SC
func (executor *bridgeExecutor) GetLastExecutedEthBatchIDFromElrond(ctx context.Context) (uint64, error) {
    return executor.elrondClient.GetLastExecutedEthBatchID(ctx)
}

// VerifyLastDepositNonceExecutedOnEthereumBatch will check the deposit nonces from the fetched batch from Ethereum client
func (executor *bridgeExecutor) VerifyLastDepositNonceExecutedOnEthereumBatch(ctx context.Context) error {
    if executor.batch == nil {
        return ErrNilBatch
    }

    lastNonce, err := executor.elrondClient.GetLastExecutedEthTxID(ctx)
    if err != nil {
        return err
    }

    return executor.verifyDepositNonces(lastNonce)
}

func (executor *bridgeExecutor) verifyDepositNonces(lastNonce uint64) error {
    startNonce := lastNonce + 1
    for _, dt := range executor.batch.Deposits {
        if dt.Nonce != startNonce {
            return fmt.Errorf("%w for deposit %s, expected: %d", ErrInvalidDepositNonce, dt.String(), startNonce)
        }

        startNonce++
    }

    return nil
}

// GetAndStoreActionIDForProposeTransferOnElrond fetches the action ID for ProposeTransfer by using the stored batch. Returns and stores the action ID
func (executor *bridgeExecutor) GetAndStoreActionIDForProposeTransferOnElrond(ctx context.Context) (uint64, error) {
    if executor.batch == nil {
        return InvalidActionID, ErrNilBatch
    }

    actionID, err := executor.elrondClient.GetActionIDForProposeTransfer(ctx, executor.batch)
    if err != nil {
        return InvalidActionID, err
    }

    executor.actionID = actionID

    return actionID, nil
}

// GetAndStoreActionIDForProposeSetStatusFromElrond fetches the action ID for SetStatus by using the stored batch. Returns and stores the action ID
func (executor *bridgeExecutor) GetAndStoreActionIDForProposeSetStatusFromElrond(ctx context.Context) (uint64, error) {
    if executor.batch == nil {
        return InvalidActionID, ErrNilBatch
    }

    actionID, err := executor.elrondClient.GetActionIDForSetStatusOnPendingTransfer(ctx, executor.batch)
    if err != nil {
        return InvalidActionID, err
    }

    executor.actionID = actionID

    return actionID, nil
}

// GetStoredActionID will return the stored action ID
func (executor *bridgeExecutor) GetStoredActionID() uint64 {
    return executor.actionID
}

// WasTransferProposedOnElrond checks if the transfer was proposed on Elrond
func (executor *bridgeExecutor) WasTransferProposedOnElrond(ctx context.Context) (bool, error) {
    if executor.batch == nil {
        return false, ErrNilBatch
    }

    return executor.elrondClient.WasProposedTransfer(ctx, executor.batch)
}

// ProposeTransferOnElrond will propose the transfer on Elrond
func (executor *bridgeExecutor) ProposeTransferOnElrond(ctx context.Context) error {
    if executor.batch == nil {
        return ErrNilBatch
    }

    hash, err := executor.elrondClient.ProposeTransfer(ctx, executor.batch)
    if err != nil {
        return err
    }

    executor.log.Info("proposed transfer", "hash", hash,
        "batch ID", executor.batch.ID, "action ID", executor.actionID)

    return nil
}

// WasSetStatusProposedOnElrond checks if set status was proposed on Elrond
func (executor *bridgeExecutor) WasSetStatusProposedOnElrond(ctx context.Context) (bool, error) {
    if executor.batch == nil {
        return false, ErrNilBatch
    }

    return executor.elrondClient.WasProposedSetStatus(ctx, executor.batch)
}

// ProposeSetStatusOnElrond will propose set status on Elrond
func (executor *bridgeExecutor) ProposeSetStatusOnElrond(ctx context.Context) error {
    if executor.batch == nil {
        return ErrNilBatch
    }

    hash, err := executor.elrondClient.ProposeSetStatus(ctx, executor.batch)
    if err != nil {
        return err
    }

    executor.log.Info("proposed set status", "hash", hash,
        "batch ID", executor.batch.ID, "action ID", executor.actionID)

    return nil
}

// WasActionSignedOnElrond returns true if the current relayer already signed the action
func (executor *bridgeExecutor) WasActionSignedOnElrond(ctx context.Context) (bool, error) {
    return executor.elrondClient.WasExecuted(ctx, executor.actionID)
}

// SignActionOnElrond will call the Elrond client to generate and send the signature
func (executor *bridgeExecutor) SignActionOnElrond(ctx context.Context) error {
    hash, err := executor.elrondClient.Sign(ctx, executor.actionID)
    if err != nil {
        return err
    }

    executor.log.Info("signed proposed transfer", "hash", hash, "action ID", executor.actionID)

    return nil
}

// IsQuorumReachedOnElrond will return true if the proposed transfer reached the set quorum
func (executor *bridgeExecutor) IsQuorumReachedOnElrond(ctx context.Context) (bool, error) {
    return executor.elrondClient.QuorumReached(ctx, executor.actionID)
}

// WasActionPerformedOnElrond will return true if the action was already performed
func (executor *bridgeExecutor) WasActionPerformedOnElrond(ctx context.Context) (bool, error) {
    return executor.elrondClient.WasExecuted(ctx, executor.actionID)
}

// PerformActionOnElrond will send the perform-action transaction on the Elrond chain
func (executor *bridgeExecutor) PerformActionOnElrond(ctx context.Context) error {
    if executor.batch == nil {
        return ErrNilBatch
    }

    hash, err := executor.elrondClient.PerformAction(ctx, executor.actionID, executor.batch)
    if err != nil {
        return err
    }

    executor.log.Info("sent perform action transaction", "hash", hash,
        "batch ID", executor.batch.ID, "action ID", executor.actionID)

    return nil
}

// ProcessMaxRetriesOnElrond checks if the retries on Elrond were reached and increments the counter
func (executor *bridgeExecutor) ProcessMaxRetriesOnElrond() bool {
    maxNumberOfRetries := executor.elrondClient.GetMaxNumberOfRetriesOnQuorumReached()
    if executor.retriesOnElrond < maxNumberOfRetries {
        executor.retriesOnElrond++
        return false
    }

    return true
}

// ResetRetriesCountOnElrond resets the number of retries on Elrond
func (executor *bridgeExecutor) ResetRetriesCountOnElrond() {
    executor.retriesOnElrond = 0
}

// MyTurnAsLeaderOnEthereum returns true if the current relayer node is the leader on Elrond
func (executor *bridgeExecutor) MyTurnAsLeaderOnEthereum() bool {
    return executor.topologyProviderOnEthereum.MyTurnAsLeader()
}

// GetAndStoreBatchFromEthereum will fetch and store the batch from the ethereum client
func (executor *bridgeExecutor) GetAndStoreBatchFromEthereum(ctx context.Context, nonce uint64) error {
    batch, err := executor.ethereumClient.GetBatch(ctx, nonce)
    // TODO add error filtering here
    if err != nil {
        return err
    }

    executor.batch = batch

    return nil
}

// WasTransferPerformedOnEthereum will return true if the batch was performed on Ethereum
func (executor *bridgeExecutor) WasTransferPerformedOnEthereum(ctx context.Context) (bool, error) {
    if executor.batch == nil {
        return false, ErrNilBatch
    }

    return executor.ethereumClient.WasExecuted(ctx, executor.batch.ID)
}

// SignTransferOnEthereum will generate the message hash for batch and broadcast the signature
func (executor *bridgeExecutor) SignTransferOnEthereum(ctx context.Context) error {
    if executor.batch == nil {
        return ErrNilBatch
    }

    hash, err := executor.ethereumClient.GenerateMessageHash(executor.batch)
    if err != nil {
        return err
    }

    executor.log.Info("generated message hash on Ethereum", hash,
        "batch ID", executor.batch.ID)

    executor.msgHash = hash
    executor.ethereumClient.BroadcastSignatureForMessageHash(hash)
    return nil
}

// PerformTransferOnEthereum will transfer a batch to Ethereum
func (executor *bridgeExecutor) PerformTransferOnEthereum(ctx context.Context) error {
    if executor.batch == nil {
        return ErrNilBatch
    }

    quorumSize, err := executor.ethereumClient.GetQuorumSize(ctx)
    if err != nil {
        return err
    }

    hash, err := executor.ethereumClient.ExecuteTransfer(ctx, executor.msgHash, executor.batch, int(quorumSize.Int64()))
    if err != nil {
        return err
    }

    executor.log.Info("sent execute transfer", "hash", hash,
        "batch ID", executor.batch.ID, "action ID")

    return nil
}

// IsQuorumReachedOnEthereum will return true if the proposed transfer reached the set quorum
func (executor *bridgeExecutor) IsQuorumReachedOnEthereum(ctx context.Context) (bool, error) {
    return executor.ethereumClient.IsQuorumReached(ctx, executor.msgHash)
}

// ProcessMaxRetriesOnEthereum checks if the retries on Ethereum were reached and increments the counter
func (executor *bridgeExecutor) ProcessMaxRetriesOnEthereum() bool {
    maxNumberOfRetries := executor.ethereumClient.GetMaxNumberOfRetriesOnQuorumReached()
    if executor.retriesOnEthereum < maxNumberOfRetries {
        executor.retriesOnEthereum++
        return false
    }

    return true
}

// ResetRetriesCountOnEthereum resets the number of retries on Ethereum
func (executor *bridgeExecutor) ResetRetriesCountOnEthereum() {
    executor.retriesOnEthereum = 0
}

// IsInterfaceNil returns true if there is no value under the interface
func (executor *bridgeExecutor) IsInterfaceNil() bool {
    return executor == nil
}
