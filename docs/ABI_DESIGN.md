# Application Blockchain Interface (ABI) v2.0 Design

## Executive Summary

This document presents a comprehensive redesign of the Application Blockchain Interface from first principles. The design supports the three-tier architecture (Glueberry P2P → Looseberry DAG Mempool → Blockberry Consensus) while providing clean separation of concerns and maximum flexibility for application developers.

## Design Philosophy

### Core Principles

1. **Layered Independence**: Each layer (P2P, Mempool, Consensus, Application) operates independently with well-defined interfaces
2. **Callback Inversion**: Rather than the framework calling the application, the application registers handlers that the framework invokes
3. **Asynchronous by Default**: All operations support async execution with synchronization points
4. **Fail-Closed Security**: Default behaviors reject/deny; applications must explicitly enable acceptance
5. **Zero-Copy Where Possible**: Minimize data copying across boundaries
6. **Deterministic Execution**: Same inputs produce same outputs across all nodes

### Design Goals

- **Composable**: Mix and match components (different mempools, consensus algorithms)
- **Testable**: Each layer can be unit tested in isolation
- **Observable**: Rich metrics and tracing at every boundary
- **Upgradeable**: Protocol versioning for smooth upgrades

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           APPLICATION LAYER                              │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │                      Application Interface                          ││
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐   ││
│  │  │ TxValidator │ │ BlockExec   │ │ StateQuery  │ │ Lifecycle   │   ││
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘   ││
│  └─────────────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────────────┤
│                          CONSENSUS LAYER                                 │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │                   ConsensusEngine Interface                         ││
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐   ││
│  │  │ BFT Engine  │ │ DAG Engine  │ │ PoS Engine  │ │ Custom...   │   ││
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘   ││
│  └─────────────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────────────┤
│                           MEMPOOL LAYER                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │                    MempoolEngine Interface                          ││
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐   ││
│  │  │ Simple FIFO │ │ Priority    │ │ DAG (Loose) │ │ Custom...   │   ││
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘   ││
│  └─────────────────────────────────────────────────────────────────────┘│
├─────────────────────────────────────────────────────────────────────────┤
│                           NETWORK LAYER                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │                    Glueberry P2P Interface                          ││
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐   ││
│  │  │ Connection  │ │ Stream Mgr  │ │ Crypto      │ │ Discovery   │   ││
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘   ││
│  └─────────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Part 1: Application Interface

The Application Interface is the primary contract between the blockchain framework and application-specific logic.

### 1.1 Core Application Interface

```go
// Application is the main interface applications must implement.
// All methods are invoked by the framework; applications respond.
type Application interface {
    // Lifecycle
    Info() ApplicationInfo
    InitChain(genesis *Genesis) error

    // Transaction Processing (ordered from loose to strict)
    CheckTx(ctx context.Context, tx *Transaction) *TxCheckResult

    // Block Execution (strict ordering)
    BeginBlock(ctx context.Context, block *BlockHeader) error
    ExecuteTx(ctx context.Context, tx *Transaction) *TxExecResult
    EndBlock(ctx context.Context) *EndBlockResult
    Commit(ctx context.Context) *CommitResult

    // State Access
    Query(ctx context.Context, req *QueryRequest) *QueryResponse
}

// ApplicationInfo provides metadata about the application.
type ApplicationInfo struct {
    Name        string
    Version     string
    AppHash     []byte           // Current application state hash
    Height      uint64           // Last committed height
    Metadata    map[string]any   // App-specific metadata
}

// Genesis contains chain initialization data.
type Genesis struct {
    ChainID        string
    GenesisTime    time.Time
    ConsensusParams *ConsensusParams
    Validators     []Validator
    AppState       []byte           // App-specific genesis state (opaque)
}
```

### 1.2 Transaction Processing

```go
// Transaction represents a blockchain transaction.
type Transaction struct {
    Hash      []byte            // SHA-256 hash
    Data      []byte            // Raw transaction bytes (opaque to framework)
    Sender    []byte            // Optional: extracted sender (for dedup)
    Nonce     uint64            // Optional: sequence number (for ordering)
    GasLimit  uint64            // Optional: max computation units
    GasPrice  uint64            // Optional: price per unit
    Priority  int64             // Derived priority (higher = more important)
    Metadata  map[string]any    // App-populated metadata
}

// CheckTxMode indicates the context of transaction validation.
type CheckTxMode int

const (
    CheckTxNew       CheckTxMode = iota  // New transaction from client/peer
    CheckTxRecheck                        // Re-validate after block commit
    CheckTxRecovery                       // Recovery from crash/restart
)

// TxCheckResult is returned from CheckTx.
type TxCheckResult struct {
    Code       ResultCode        // 0 = success
    Error      error             // Human-readable error (if Code != 0)
    GasWanted  uint64            // Estimated gas needed
    Priority   int64             // Transaction priority for ordering
    Sender     []byte            // Extracted sender (for mempool dedup)
    Nonce      uint64            // Extracted nonce (for ordering)
    Data       []byte            // Optional: modified transaction

    // Mempool behavior hints
    RecheckAfter uint64          // Suggest re-check after this height
    ExpireAfter  time.Duration   // Suggest expiration time
}

// TxExecResult is returned from ExecuteTx during block execution.
type TxExecResult struct {
    Code       ResultCode
    Error      error
    GasUsed    uint64
    Events     []Event           // Emitted events
    Data       []byte            // Return data

    // State changes (optional, for indexing)
    StateChanges []StateChange
}

// ResultCode for transaction processing.
type ResultCode uint32

const (
    CodeOK                  ResultCode = 0
    CodeUnknownError        ResultCode = 1
    CodeInvalidTx           ResultCode = 2
    CodeInsufficientFunds   ResultCode = 3
    CodeInvalidNonce        ResultCode = 4
    CodeInsufficientGas     ResultCode = 5
    CodeNotAuthorized       ResultCode = 6
    CodeInvalidState        ResultCode = 7
    // 100-999: App-specific codes
)
```

### 1.3 Block Execution

```go
// BlockHeader contains block metadata passed to BeginBlock.
type BlockHeader struct {
    Height          uint64
    Time            time.Time
    PrevHash        []byte
    ProposerAddress []byte

    // Consensus metadata (opaque to application)
    ConsensusData   []byte

    // Evidence of misbehavior (for slashing)
    Evidence        []Evidence

    // Validator updates from last block
    LastValidators  []Validator
}

// EndBlockResult contains application responses to block finalization.
type EndBlockResult struct {
    ValidatorUpdates []ValidatorUpdate  // Changes to validator set
    ConsensusParams  *ConsensusParams   // Changes to consensus params (rare)
    Events           []Event            // Block-level events
}

// CommitResult contains the result of committing state.
type CommitResult struct {
    AppHash     []byte    // New state root hash
    RetainHeight uint64   // Suggested minimum height to retain
}

// Event represents an application event.
type Event struct {
    Type       string
    Attributes []Attribute
}

type Attribute struct {
    Key   string
    Value []byte
    Index bool   // Should this be indexed?
}
```

### 1.4 State Query Interface

```go
// QueryRequest for reading application state.
type QueryRequest struct {
    Path    string    // Query path (e.g., "/accounts/{address}")
    Data    []byte    // Query-specific data
    Height  uint64    // Historical height (0 = latest)
    Prove   bool      // Include Merkle proof
}

// QueryResponse from Query.
type QueryResponse struct {
    Code      ResultCode
    Error     error
    Key       []byte
    Value     []byte
    Proof     *Proof    // Merkle proof (if requested)
    Height    uint64    // Height at which query was executed
}

// Proof represents a Merkle proof.
type Proof struct {
    Ops []ProofOp   // Proof operations
}

type ProofOp struct {
    Type string
    Key  []byte
    Data []byte
}
```

---

## Part 2: Extended Application Interfaces

Applications can implement additional interfaces for richer functionality.

### 2.1 Snapshot Interface (State Sync)

```go
// SnapshotApplication supports state sync via snapshots.
type SnapshotApplication interface {
    Application

    // List available snapshots
    ListSnapshots() []*Snapshot

    // Load a specific snapshot chunk
    LoadSnapshotChunk(height uint64, format uint32, chunk uint32) ([]byte, error)

    // Offer a snapshot for restoration
    OfferSnapshot(snapshot *Snapshot) OfferResult

    // Apply a snapshot chunk
    ApplySnapshotChunk(chunk []byte, index uint32) ApplyResult
}

type Snapshot struct {
    Height  uint64
    Format  uint32   // Snapshot format version
    Chunks  uint32   // Number of chunks
    Hash    []byte   // Snapshot hash
    Metadata []byte  // App-specific metadata
}

type OfferResult uint32
const (
    OfferAccept       OfferResult = iota  // Begin restoration
    OfferAbort                             // Abort restoration
    OfferReject                            // Reject, try another
    OfferRejectFormat                      // Reject format, try another
    OfferRejectSender                      // Reject sender, try another
)

type ApplyResult uint32
const (
    ApplyAccept        ApplyResult = iota  // Chunk applied
    ApplyAbort                              // Abort restoration
    ApplyRetry                              // Retry chunk
    ApplyRetrySnapshot                      // Retry entire snapshot
    ApplyRejectSnapshot                     // Reject snapshot
)
```

### 2.2 Prepare/Process Proposal (Leader Selection)

```go
// ProposerApplication supports custom block proposal logic.
type ProposerApplication interface {
    Application

    // PrepareProposal allows the app to modify a proposed block before broadcasting.
    // The proposer calls this before sending the proposal.
    PrepareProposal(ctx context.Context, req *PrepareRequest) *PrepareResponse

    // ProcessProposal allows the app to accept/reject a received proposal.
    // Non-proposers call this before voting.
    ProcessProposal(ctx context.Context, req *ProcessRequest) *ProcessResponse
}

type PrepareRequest struct {
    MaxTxBytes   int64
    Txs          [][]byte         // Candidate transactions
    LocalLastCommit CommitInfo    // Info about last commit
    Misbehavior  []Misbehavior    // Evidence
    Height       uint64
    Time         time.Time
    ProposerAddress []byte
}

type PrepareResponse struct {
    Txs [][]byte   // Final ordered transactions for block
}

type ProcessRequest struct {
    Txs          [][]byte
    ProposedLastCommit CommitInfo
    Misbehavior  []Misbehavior
    Hash         []byte
    Height       uint64
    Time         time.Time
    ProposerAddress []byte
}

type ProcessResponse struct {
    Status ProcessStatus
}

type ProcessStatus uint32
const (
    ProcessAccept  ProcessStatus = iota  // Valid proposal
    ProcessReject                         // Invalid proposal
)
```

### 2.3 Finality Extensions

```go
// FinalityApplication supports extended finality information.
type FinalityApplication interface {
    Application

    // ExtendVote allows the app to add data to consensus votes.
    ExtendVote(ctx context.Context, req *ExtendVoteRequest) *ExtendVoteResponse

    // VerifyVoteExtension validates vote extensions from other validators.
    VerifyVoteExtension(ctx context.Context, req *VerifyVoteExtRequest) *VerifyVoteExtResponse

    // FinalizeBlock is called when a block achieves finality.
    FinalizeBlock(ctx context.Context, req *FinalizeBlockRequest) *FinalizeBlockResponse
}

type ExtendVoteRequest struct {
    Hash   []byte
    Height uint64
    Time   time.Time
}

type ExtendVoteResponse struct {
    VoteExtension []byte   // App-specific data to include in vote
}

type VerifyVoteExtRequest struct {
    Hash          []byte
    ValidatorAddress []byte
    Height        uint64
    VoteExtension []byte
}

type VerifyVoteExtResponse struct {
    Status VerifyStatus
}

type VerifyStatus uint32
const (
    VerifyAccept  VerifyStatus = iota
    VerifyReject
)

type FinalizeBlockRequest struct {
    Txs         [][]byte
    DecidedLastCommit CommitInfo
    Misbehavior []Misbehavior
    Hash        []byte
    Height      uint64
    Time        time.Time
    ProposerAddress []byte
}

type FinalizeBlockResponse struct {
    Events            []Event
    TxResults         []TxExecResult
    ValidatorUpdates  []ValidatorUpdate
    ConsensusParams   *ConsensusParams
    AppHash           []byte
}
```

---

## Part 3: Mempool Interface

### 3.1 Core Mempool Engine

```go
// MempoolEngine is the interface for transaction pool implementations.
type MempoolEngine interface {
    Component

    // AddTx adds a transaction to the mempool.
    // Returns immediately; use callback for async result.
    AddTx(tx *Transaction, callback TxCallback) error

    // RemoveTxs removes transactions (after commit).
    RemoveTxs(hashes [][]byte)

    // ReapTxs returns transactions for block proposal.
    ReapTxs(opts ReapOptions) []*Transaction

    // Size returns current mempool state.
    Size() MempoolSize

    // HasTx checks if transaction exists.
    HasTx(hash []byte) bool

    // Flush removes all transactions.
    Flush()

    // Update is called after a block is committed.
    Update(height uint64, txs []*Transaction) error

    // SetTxValidator sets the transaction validation function.
    SetTxValidator(validator TxValidator)
}

type TxCallback func(result *TxCheckResult)

type TxValidator func(ctx context.Context, tx *Transaction) *TxCheckResult

type ReapOptions struct {
    MaxBytes    int64
    MaxTxs      int
    MinPriority int64
}

type MempoolSize struct {
    TxCount   int
    ByteCount int64
}
```

### 3.2 DAG Mempool Extension (Looseberry Integration)

```go
// DAGMempoolEngine extends MempoolEngine for DAG-based ordering.
type DAGMempoolEngine interface {
    MempoolEngine

    // ReapCertifiedBatches returns ordered, certified transaction batches.
    ReapCertifiedBatches(maxBytes int64) []*CertifiedBatch

    // NotifyCommitted informs the DAG of committed rounds.
    NotifyCommitted(round uint64)

    // UpdateValidatorSet updates the set of validators for certification.
    UpdateValidatorSet(validators []Validator)

    // CurrentRound returns the current DAG round.
    CurrentRound() uint64

    // DAGMetrics returns DAG-specific metrics.
    DAGMetrics() *DAGMetrics
}

// CertifiedBatch represents an ordered batch with quorum certification.
type CertifiedBatch struct {
    Round       uint64
    Validator   []byte        // Producing validator
    Txs         []*Transaction
    Certificate *Certificate  // Quorum signatures proving ordering
}

// Certificate proves batch ordering via quorum signatures.
type Certificate struct {
    Round      uint64
    BatchHash  []byte
    Signatures []ValidatorSig
}

type ValidatorSig struct {
    ValidatorAddress []byte
    Signature        []byte
}

type DAGMetrics struct {
    CurrentRound    uint64
    CommittedRound  uint64
    PendingBatches  int
    CertifiedBatches int
    ValidatorCount  int
}
```

### 3.3 Network-Aware Mempool

```go
// NetworkAwareMempoolEngine can declare custom network streams.
type NetworkAwareMempoolEngine interface {
    MempoolEngine

    // StreamConfigs returns stream configurations for the network layer.
    StreamConfigs() []StreamConfig

    // SetNetwork provides the network interface.
    SetNetwork(network MempoolNetwork)

    // HandleStreamMessage processes messages from custom streams.
    HandleStreamMessage(stream string, peerID []byte, data []byte) error
}

type StreamConfig struct {
    Name        string
    Direction   StreamDirection
    Priority    int
    BufferSize  int
}

type StreamDirection int
const (
    StreamBidirectional StreamDirection = iota
    StreamInbound
    StreamOutbound
)

// MempoolNetwork provides network operations to the mempool.
type MempoolNetwork interface {
    // Broadcast sends data to all peers.
    Broadcast(stream string, data []byte) error

    // Send sends data to a specific peer.
    Send(peerID []byte, stream string, data []byte) error

    // PeerCount returns the number of connected peers.
    PeerCount() int

    // ValidatorPeers returns connected validator peer IDs.
    ValidatorPeers() [][]byte
}
```

---

## Part 4: Consensus Interface

### 4.1 Core Consensus Engine

```go
// ConsensusEngine is the interface for consensus implementations.
type ConsensusEngine interface {
    Component
    Named

    // Initialize sets up the engine with its dependencies.
    Initialize(deps ConsensusDependencies) error

    // State queries
    Height() uint64
    Round() uint32
    State() ConsensusState

    // Validator info
    IsValidator() bool
    ValidatorSet() []Validator

    // Message handling
    HandleMessage(peerID []byte, msgType ConsensusMessageType, data []byte) error
}

type ConsensusDependencies struct {
    Network     ConsensusNetwork
    BlockStore  BlockStore
    StateStore  StateStore
    Mempool     MempoolEngine
    Application Application
    Signer      Signer
    Config      *ConsensusConfig
    Logger      Logger
    Metrics     Metrics
}

type ConsensusState int
const (
    ConsensusStateIdle ConsensusState = iota
    ConsensusStateProposing
    ConsensusStatePrevoting
    ConsensusStatePrecommitting
    ConsensusStateCommitting
    ConsensusStateCatchup
)

type ConsensusMessageType int
const (
    MsgTypeProposal ConsensusMessageType = iota
    MsgTypePrevote
    MsgTypePrecommit
    MsgTypeCommit
    MsgTypeBlock
    MsgTypeBlockPart
    MsgTypeVoteSetBits
    MsgTypeNewRoundStep
    MsgTypeHasVote
)
```

### 4.2 BFT Consensus Extension

```go
// BFTConsensusEngine extends ConsensusEngine for BFT protocols.
type BFTConsensusEngine interface {
    ConsensusEngine

    // Block production
    ProposeBlock(ctx context.Context) (*BlockProposal, error)

    // Message handling
    HandleProposal(proposal *Proposal) error
    HandlePrevote(vote *Vote) error
    HandlePrecommit(vote *Vote) error
    HandleCommit(commit *Commit) error

    // Timeout handling
    OnTimeout(height uint64, round uint32, step TimeoutStep) error

    // Evidence
    ReportMisbehavior(evidence Evidence) error
}

type BlockProposal struct {
    Height      uint64
    Round       uint32
    Block       *Block
    POLRound    int32   // Proof-of-Lock round (-1 if none)
    Signature   []byte
}

type Proposal struct {
    Type        ConsensusMessageType
    Height      uint64
    Round       uint32
    BlockHash   []byte
    POLRound    int32
    Timestamp   time.Time
    Signature   []byte
}

type Vote struct {
    Type             ConsensusMessageType  // Prevote or Precommit
    Height           uint64
    Round            uint32
    BlockHash        []byte     // nil = nil vote
    Timestamp        time.Time
    ValidatorAddress []byte
    ValidatorIndex   int32
    Signature        []byte
    Extension        []byte     // Vote extension (optional)
    ExtensionSig     []byte
}

type Commit struct {
    Height     uint64
    Round      uint32
    BlockHash  []byte
    Signatures []CommitSig
}

type CommitSig struct {
    Flag             BlockIDFlag
    ValidatorAddress []byte
    Timestamp        time.Time
    Signature        []byte
}

type BlockIDFlag uint8
const (
    BlockIDFlagAbsent BlockIDFlag = iota
    BlockIDFlagCommit
    BlockIDFlagNil
)

type TimeoutStep int
const (
    TimeoutPropose TimeoutStep = iota
    TimeoutPrevote
    TimeoutPrecommit
)
```

### 4.3 DAG Consensus Extension

```go
// DAGConsensusEngine extends ConsensusEngine for DAG-based protocols.
type DAGConsensusEngine interface {
    ConsensusEngine

    // DAG operations
    AddVertex(vertex *DAGVertex) error
    GetVertex(hash []byte) (*DAGVertex, error)
    GetVerticesByRound(round uint64) []*DAGVertex

    // Certificate operations
    HandleCertificate(cert *Certificate) error
    GetCertificate(round uint64, validator []byte) (*Certificate, error)

    // Ordering
    GetOrderedVertices(fromRound, toRound uint64) []*DAGVertex
    CommittedRound() uint64

    // Wave/Leader operations (for Tusk/Bullshark)
    GetWaveLeader(wave uint64) ([]byte, error)
    IsLeaderVertex(vertex *DAGVertex) bool
}

type DAGVertex struct {
    Round      uint64
    Source     []byte           // Validator that created this
    Parents    [][]byte         // Hashes of parent vertices
    Payload    []byte           // Transaction batch or other data
    Timestamp  time.Time
    Signature  []byte
    Hash       []byte           // Computed hash
}
```

### 4.4 Stream-Aware Consensus

```go
// StreamAwareConsensusEngine can declare custom network streams.
type StreamAwareConsensusEngine interface {
    ConsensusEngine

    // StreamConfigs returns stream configurations.
    StreamConfigs() []StreamConfig

    // HandleStreamMessage processes messages from custom streams.
    HandleStreamMessage(stream string, peerID []byte, data []byte) error
}
```

---

## Part 5: Event System

### 5.1 Event Bus

```go
// EventBus provides pub/sub for system events.
type EventBus interface {
    Component

    // Subscribe to events matching the query.
    Subscribe(ctx context.Context, subscriber string, query Query) (<-chan Event, error)

    // Unsubscribe from events.
    Unsubscribe(ctx context.Context, subscriber string, query Query) error

    // UnsubscribeAll removes all subscriptions for a subscriber.
    UnsubscribeAll(ctx context.Context, subscriber string) error

    // Publish sends an event to all matching subscribers.
    Publish(ctx context.Context, event Event) error

    // PublishWithTimeout publishes with a timeout for slow subscribers.
    PublishWithTimeout(ctx context.Context, event Event, timeout time.Duration) error
}

// Query for filtering events.
type Query interface {
    Matches(event Event) bool
    String() string
}

// Common event types
const (
    EventNewBlock       = "NewBlock"
    EventNewBlockHeader = "NewBlockHeader"
    EventTx             = "Tx"
    EventVote           = "Vote"
    EventCommit         = "Commit"
    EventValidatorSetUpdates = "ValidatorSetUpdates"
    EventEvidence       = "Evidence"

    // Mempool events
    EventTxAdded        = "TxAdded"
    EventTxRemoved      = "TxRemoved"
    EventTxRejected     = "TxRejected"

    // Network events
    EventPeerConnected    = "PeerConnected"
    EventPeerDisconnected = "PeerDisconnected"
    EventPeerMisbehavior  = "PeerMisbehavior"

    // Sync events
    EventSyncStarted    = "SyncStarted"
    EventSyncProgress   = "SyncProgress"
    EventSyncCompleted  = "SyncCompleted"
)
```

### 5.2 Callback Registry

```go
// CallbackRegistry manages application callbacks.
type CallbackRegistry struct {
    // Transaction callbacks
    OnTxReceived   func(peerID []byte, tx *Transaction)
    OnTxValidated  func(tx *Transaction, result *TxCheckResult)
    OnTxAdded      func(tx *Transaction)
    OnTxRemoved    func(hash []byte, reason TxRemovalReason)
    OnTxBroadcast  func(tx *Transaction, peerCount int)

    // Block callbacks
    OnBlockProposed    func(height uint64, hash []byte, txCount int)
    OnBlockReceived    func(peerID []byte, height uint64, hash []byte)
    OnBlockValidated   func(height uint64, hash []byte, err error)
    OnBlockCommitted   func(height uint64, hash []byte, appHash []byte)

    // Consensus callbacks
    OnConsensusStateChange func(oldState, newState ConsensusState)
    OnRoundChange          func(height uint64, round uint32)
    OnVoteReceived         func(vote *Vote)
    OnProposalReceived     func(proposal *Proposal)

    // Validator callbacks
    OnValidatorSetChange func(height uint64, validators []Validator)
    OnSlashingEvent      func(validator []byte, reason string, amount uint64)

    // Network callbacks
    OnPeerConnected    func(peerID []byte, isOutbound bool)
    OnPeerDisconnected func(peerID []byte)
    OnPeerScoreChanged func(peerID []byte, oldScore, newScore int)
}

type TxRemovalReason int
const (
    TxRemovalCommitted TxRemovalReason = iota
    TxRemovalExpired
    TxRemovalReplaced
    TxRemovalInvalid
    TxRemovalEvicted
)
```

---

## Part 6: Component Lifecycle

### 6.1 Component Interface

```go
// Component is the base interface for all manageable components.
type Component interface {
    Start() error
    Stop() error
    IsRunning() bool
}

// Named components can report their name.
type Named interface {
    Name() string
}

// HealthChecker components can report health status.
type HealthChecker interface {
    Health() HealthStatus
}

type HealthStatus struct {
    Status    Health
    Message   string
    Details   map[string]any
    Timestamp time.Time
}

type Health int
const (
    HealthUnknown Health = iota
    HealthHealthy
    HealthDegraded
    HealthUnhealthy
)

// Dependent components declare their dependencies.
type Dependent interface {
    Dependencies() []string
}

// LifecycleAware components receive lifecycle notifications.
type LifecycleAware interface {
    OnStart() error
    OnStop() error
}

// Resettable components can be reset to initial state.
type Resettable interface {
    Reset() error
}
```

### 6.2 Service Registry

```go
// ServiceRegistry manages component lifecycle and dependencies.
type ServiceRegistry interface {
    // Register a component.
    Register(name string, component Component) error

    // Get a component by name.
    Get(name string) (Component, error)

    // MustGet panics if component not found.
    MustGet(name string) Component

    // StartAll starts all components in dependency order.
    StartAll() error

    // StopAll stops all components in reverse dependency order.
    StopAll() error

    // Health returns aggregate health of all components.
    Health() map[string]HealthStatus
}
```

---

## Part 7: Storage Interfaces

### 7.1 Block Store

```go
// BlockStore persists blocks and provides retrieval.
type BlockStore interface {
    Component

    // Save a block and its metadata.
    SaveBlock(block *Block, parts *PartSet, commit *Commit) error

    // Load operations
    LoadBlock(height uint64) (*Block, error)
    LoadBlockMeta(height uint64) (*BlockMeta, error)
    LoadBlockPart(height uint64, index int) (*Part, error)
    LoadBlockCommit(height uint64) (*Commit, error)
    LoadSeenCommit(height uint64) (*Commit, error)

    // Height queries
    Base() uint64    // Lowest available height
    Height() uint64  // Highest stored height

    // Pruning
    PruneBlocks(retainHeight uint64) (uint64, error)
}

type Block struct {
    Header     BlockHeader
    Data       BlockData
    Evidence   EvidenceData
    LastCommit *Commit
}

type BlockData struct {
    Txs [][]byte
}

type BlockMeta struct {
    BlockID   BlockID
    BlockSize int64
    Header    BlockHeader
    NumTxs    int64
}

type BlockID struct {
    Hash          []byte
    PartSetHeader PartSetHeader
}
```

### 7.2 State Store

```go
// StateStore manages consensus and application state.
type StateStore interface {
    Component

    // Consensus state
    LoadState() (*ConsensusState, error)
    SaveState(state *ConsensusState) error

    // Validator sets
    LoadValidators(height uint64) (*ValidatorSet, error)
    SaveValidators(height uint64, validators *ValidatorSet) error

    // Consensus params
    LoadConsensusParams(height uint64) (*ConsensusParams, error)
    SaveConsensusParams(height uint64, params *ConsensusParams) error

    // ABCI responses (for replay)
    LoadABCIResponses(height uint64) (*ABCIResponses, error)
    SaveABCIResponses(height uint64, responses *ABCIResponses) error

    // Pruning
    PruneStates(retainHeight uint64) error
}
```

### 7.3 Evidence Store

```go
// EvidenceStore tracks misbehavior evidence.
type EvidenceStore interface {
    Component

    // Add new evidence.
    AddEvidence(evidence Evidence) error

    // Check if evidence already exists.
    HasEvidence(evidence Evidence) bool

    // Get pending evidence for block inclusion.
    PendingEvidence(maxBytes int64) []Evidence

    // Mark evidence as committed.
    MarkEvidenceCommitted(evidence Evidence) error

    // Check if evidence is expired.
    IsExpired(evidence Evidence, currentHeight uint64, currentTime time.Time) bool
}

type Evidence interface {
    Hash() []byte
    Height() uint64
    Time() time.Time
    ValidateBasic() error
    String() string
}
```

---

## Part 8: Network Interface

### 8.1 Network Abstraction

```go
// Network provides P2P communication capabilities.
type Network interface {
    Component

    // Identity
    PeerID() []byte
    PublicKey() []byte
    Addresses() []string

    // Peer management
    Connect(peerID []byte) error
    Disconnect(peerID []byte) error
    Peers() []PeerInfo
    PeerCount() int

    // Messaging
    Send(peerID []byte, stream string, data []byte) error
    Broadcast(stream string, data []byte) error

    // Event subscription
    ConnectionEvents() <-chan ConnectionEvent
    Messages(stream string) <-chan IncomingMessage

    // Stream configuration
    RegisterStream(config StreamConfig) error
}

type PeerInfo struct {
    PeerID      []byte
    PublicKey   []byte
    Addresses   []string
    IsOutbound  bool
    IsValidator bool
    Score       int
    Latency     time.Duration
    BytesSent   uint64
    BytesRecv   uint64
    ConnectedAt time.Time
}

type ConnectionEvent struct {
    PeerID    []byte
    State     ConnectionState
    Error     error
    Timestamp time.Time
}

type ConnectionState int
const (
    ConnectionDisconnected ConnectionState = iota
    ConnectionConnecting
    ConnectionConnected
    ConnectionEstablished
)

type IncomingMessage struct {
    PeerID    []byte
    Stream    string
    Data      []byte
    Timestamp time.Time
}
```

### 8.2 Peer Scoring

```go
// PeerScorer tracks peer behavior and reputation.
type PeerScorer interface {
    // Score adjustments
    AddScore(peerID []byte, score int, reason string)
    SetScore(peerID []byte, score int)
    GetScore(peerID []byte) int

    // Behavior tracking
    RecordGoodBehavior(peerID []byte, behavior string)
    RecordBadBehavior(peerID []byte, behavior string, severity int)

    // Queries
    GetBestPeers(n int) [][]byte
    GetWorstPeers(n int) [][]byte
    IsBanned(peerID []byte) bool

    // Actions
    BanPeer(peerID []byte, duration time.Duration, reason string)
    UnbanPeer(peerID []byte)
}
```

---

## Part 9: Crypto Interface

### 9.1 Signer Interface

```go
// Signer handles cryptographic signing operations.
type Signer interface {
    // Identity
    PublicKey() []byte
    Address() []byte

    // Signing
    Sign(data []byte) ([]byte, error)
    SignWithContext(ctx string, data []byte) ([]byte, error)

    // Validation
    Verify(publicKey, data, signature []byte) bool
}

// ThresholdSigner supports threshold signature schemes.
type ThresholdSigner interface {
    Signer

    // Threshold operations
    PartialSign(data []byte) (*PartialSignature, error)
    CombinePartials(partials []*PartialSignature) ([]byte, error)
    VerifyPartial(publicKey []byte, data []byte, partial *PartialSignature) bool

    // Key generation (DKG)
    GenerateShares(threshold, total int) ([]*KeyShare, error)
    CombineShares(shares []*KeyShare) error
}

type PartialSignature struct {
    SignerIndex int
    Signature   []byte
}

type KeyShare struct {
    Index int
    Share []byte
}
```

### 9.2 Key Management

```go
// KeyManager handles key storage and retrieval.
type KeyManager interface {
    // Key retrieval
    GetPrivateKey(keyType string) ([]byte, error)
    GetPublicKey(keyType string) ([]byte, error)

    // Key operations
    GenerateKey(keyType string) ([]byte, error)
    ImportKey(keyType string, data []byte) error
    ExportKey(keyType string) ([]byte, error)
    DeleteKey(keyType string) error

    // Key listing
    ListKeys() []KeyInfo
}

type KeyInfo struct {
    Type      string
    PublicKey []byte
    CreatedAt time.Time
}
```

---

## Part 10: Configuration

### 10.1 Node Configuration

```go
// NodeConfig contains all node configuration.
type NodeConfig struct {
    // Identity
    ChainID       string
    Moniker       string

    // Network
    ListenAddresses []string
    ExternalAddress string
    MaxPeers        int
    PersistentPeers []string

    // Consensus
    ConsensusType   string
    ConsensusConfig map[string]any

    // Mempool
    MempoolType   string
    MempoolConfig map[string]any

    // Storage
    DataDir       string
    BlockStoreDB  string
    StateStoreDB  string

    // Performance
    MaxBlockBytes int64
    MaxTxBytes    int64
    TimeoutCommit time.Duration

    // Pruning
    PruningStrategy string
    PruningKeepRecent uint64

    // Instrumentation
    PrometheusEnabled bool
    PrometheusAddr    string
    TracingEnabled    bool
    TracingEndpoint   string
}
```

### 10.2 Consensus Parameters

```go
// ConsensusParams define consensus rules (on-chain).
type ConsensusParams struct {
    Block     BlockParams
    Evidence  EvidenceParams
    Validator ValidatorParams
    Version   VersionParams
    ABCI      ABCIParams
}

type BlockParams struct {
    MaxBytes   int64
    MaxGas     int64
}

type EvidenceParams struct {
    MaxAgeNumBlocks int64
    MaxAgeDuration  time.Duration
    MaxBytes        int64
}

type ValidatorParams struct {
    PubKeyTypes []string
}

type VersionParams struct {
    App uint64
}

type ABCIParams struct {
    VoteExtensionsEnabled bool
}
```

---

## Part 11: Observability

### 11.1 Metrics Interface

```go
// Metrics collects operational metrics.
type Metrics interface {
    // Consensus
    ConsensusHeight(height uint64)
    ConsensusRound(round uint32)
    ConsensusProposalReceived()
    ConsensusVoteReceived(voteType string)
    ConsensusBlockCommitted(duration time.Duration)

    // Mempool
    MempoolSize(count int, bytes int64)
    MempoolTxAdded()
    MempoolTxRemoved(reason string)
    MempoolTxRejected(reason string)
    MempoolReapDuration(duration time.Duration)

    // Application
    AppBeginBlock(duration time.Duration)
    AppExecuteTx(duration time.Duration, success bool)
    AppEndBlock(duration time.Duration)
    AppCommit(duration time.Duration)
    AppQuery(duration time.Duration, path string)

    // Network
    NetworkPeers(count int)
    NetworkBytesSent(stream string, bytes int)
    NetworkBytesReceived(stream string, bytes int)
    NetworkMessageSent(stream string)
    NetworkMessageReceived(stream string)

    // Storage
    BlockStoreHeight(height uint64)
    BlockStoreSizeBytes(bytes int64)
    StateStoreCommit(duration time.Duration)
}
```

### 11.2 Tracing Interface

```go
// Tracer provides distributed tracing.
type Tracer interface {
    // Span creation
    StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span)

    // Span extraction/injection (for cross-process)
    Extract(carrier Carrier) (SpanContext, error)
    Inject(ctx SpanContext, carrier Carrier) error
}

type Span interface {
    End()
    SetAttribute(key string, value any)
    AddEvent(name string, attrs ...Attribute)
    RecordError(err error)
    SetStatus(code StatusCode, description string)
}

type SpanOption interface {
    Apply(*SpanConfig)
}

type SpanContext interface {
    TraceID() string
    SpanID() string
    IsSampled() bool
}

type StatusCode int
const (
    StatusUnset StatusCode = iota
    StatusOK
    StatusError
)
```

---

## Part 12: Integration Patterns

### 12.1 Minimal Application Example

```go
// MinimalApp demonstrates the minimum required implementation.
type MinimalApp struct {
    state     map[string][]byte
    height    uint64
    appHash   []byte
}

func (app *MinimalApp) Info() ApplicationInfo {
    return ApplicationInfo{
        Name:    "minimal",
        Version: "1.0.0",
        AppHash: app.appHash,
        Height:  app.height,
    }
}

func (app *MinimalApp) InitChain(genesis *Genesis) error {
    // Initialize state from genesis
    return nil
}

func (app *MinimalApp) CheckTx(ctx context.Context, tx *Transaction) *TxCheckResult {
    // Validate transaction (lightweight)
    return &TxCheckResult{Code: CodeOK}
}

func (app *MinimalApp) BeginBlock(ctx context.Context, block *BlockHeader) error {
    return nil
}

func (app *MinimalApp) ExecuteTx(ctx context.Context, tx *Transaction) *TxExecResult {
    // Execute transaction (update state)
    return &TxExecResult{Code: CodeOK}
}

func (app *MinimalApp) EndBlock(ctx context.Context) *EndBlockResult {
    return &EndBlockResult{}
}

func (app *MinimalApp) Commit(ctx context.Context) *CommitResult {
    // Compute and store state hash
    app.height++
    app.appHash = computeHash(app.state)
    return &CommitResult{AppHash: app.appHash}
}

func (app *MinimalApp) Query(ctx context.Context, req *QueryRequest) *QueryResponse {
    value, ok := app.state[req.Path]
    if !ok {
        return &QueryResponse{Code: CodeUnknownError}
    }
    return &QueryResponse{Code: CodeOK, Value: value}
}
```

### 12.2 DAG-Enabled Application

```go
// DAGApp demonstrates integration with DAG mempool.
type DAGApp struct {
    MinimalApp
    dagMempool DAGMempoolEngine
}

func (app *DAGApp) PrepareProposal(ctx context.Context, req *PrepareRequest) *PrepareResponse {
    // Use DAG-ordered batches instead of simple mempool reap
    batches := app.dagMempool.ReapCertifiedBatches(req.MaxTxBytes)

    var txs [][]byte
    for _, batch := range batches {
        for _, tx := range batch.Txs {
            txs = append(txs, tx.Data)
        }
    }

    return &PrepareResponse{Txs: txs}
}

func (app *DAGApp) Commit(ctx context.Context) *CommitResult {
    result := app.MinimalApp.Commit(ctx)

    // Notify DAG of committed round
    app.dagMempool.NotifyCommitted(app.dagMempool.CurrentRound())

    return result
}
```

### 12.3 Looseberry Integration

```go
// LooseberryAdapter wraps Looseberry as a DAGMempoolEngine.
type LooseberryAdapter struct {
    looseberry *looseberry.Looseberry
}

func (a *LooseberryAdapter) AddTx(tx *Transaction, callback TxCallback) error {
    err := a.looseberry.AddTx(tx.Data)
    if callback != nil {
        result := &TxCheckResult{Code: CodeOK}
        if err != nil {
            result.Code = CodeInvalidTx
            result.Error = err
        }
        callback(result)
    }
    return err
}

func (a *LooseberryAdapter) ReapCertifiedBatches(maxBytes int64) []*CertifiedBatch {
    batches := a.looseberry.ReapCertifiedBatches(maxBytes)

    result := make([]*CertifiedBatch, len(batches))
    for i, b := range batches {
        result[i] = &CertifiedBatch{
            Round:       b.Certificate.Round,
            Validator:   b.Batch.Validator,
            Txs:         convertTxs(b.Batch.Txs),
            Certificate: convertCert(b.Certificate),
        }
    }
    return result
}

func (a *LooseberryAdapter) NotifyCommitted(round uint64) {
    a.looseberry.NotifyCommitted(round)
}

func (a *LooseberryAdapter) UpdateValidatorSet(validators []Validator) {
    a.looseberry.SetValidatorSet(convertValidators(validators))
}
```

---

## Part 13: Migration Guide

### 13.1 From ABCI v1

| ABCI v1 | ABI v2 |
|---------|--------|
| `CheckTx(RequestCheckTx)` | `CheckTx(ctx, *Transaction)` |
| `BeginBlock(RequestBeginBlock)` | `BeginBlock(ctx, *BlockHeader)` |
| `DeliverTx(RequestDeliverTx)` | `ExecuteTx(ctx, *Transaction)` |
| `EndBlock(RequestEndBlock)` | `EndBlock(ctx)` |
| `Commit()` | `Commit(ctx)` |
| `Query(RequestQuery)` | `Query(ctx, *QueryRequest)` |

### 13.2 Key Changes

1. **Context Support**: All methods receive `context.Context` for cancellation
2. **Structured Types**: Rich types instead of byte slices
3. **Unified Results**: Consistent `*Result` return types
4. **Event System**: Events are part of results, not separate
5. **Optional Interfaces**: Extended functionality via interface composition

---

## Part 14: Security Considerations

### 14.1 Fail-Closed Defaults

All validators and handlers default to rejecting:

```go
// DefaultTxValidator rejects all transactions
func DefaultTxValidator(ctx context.Context, tx *Transaction) *TxCheckResult {
    return &TxCheckResult{
        Code:  CodeNotAuthorized,
        Error: errors.New("no validator configured"),
    }
}
```

### 14.2 Input Validation

All network data must be validated:

```go
func (msg *Proposal) ValidateBasic() error {
    if msg.Height == 0 {
        return errors.New("height cannot be zero")
    }
    if len(msg.BlockHash) != HashSize {
        return errors.New("invalid block hash size")
    }
    // ... additional validation
    return nil
}
```

### 14.3 Resource Limits

Configurable limits prevent resource exhaustion:

```go
type ResourceLimits struct {
    MaxTxSize       int64
    MaxBlockSize    int64
    MaxEvidenceSize int64
    MaxMsgSize      int64
    MaxPeers        int
    MaxSubscribers  int
}
```

---

## Conclusion

This ABI v2 design provides:

1. **Clean Separation**: Each layer has well-defined responsibilities
2. **Extensibility**: Optional interfaces for advanced features
3. **Composability**: Mix-and-match mempools, consensus, storage
4. **Safety**: Fail-closed defaults, comprehensive validation
5. **Observability**: Rich metrics and tracing throughout
6. **Compatibility**: Clear migration path from ABCI v1

The design supports the full range of use cases from simple single-validator chains to complex DAG-based consensus with the Looseberry integration.
