# Overview

## Workflow

- Partition is deployed -> `ISequences.New()` is called
  - `go actualizer()` is called
- `actualizer()`
  - is a goroutine
  - works for small amount of time, quickly launches flusher, then actualizer goroutine stops.
  - shutdowns `flusher` if it is running by closing `flusherCtx`, waits for `flusher` to stop
  - calls `ISeqStorage.ActualizeSequencesFromPLog(..., batcher func(batch []SeqValue, offset PLogOffset))`
    - Scan PLog from the given offset and send values to the batcher
    - batcher is callback, func of the sequencer
      - Build maxValues: max Number for each `SeqValue.Key`
  - write maxValues using `ISeqStorage.WriteValues()`
    - ??? should it `ISeqStorage.WriteNextPLogOffset`?
  - determined maxValues goes to LRU cache
  - `inprocOffset` := `ISeqStorage.ReadLastWrittenPLogOffset()`
  - `cleanpCtx` closed -> exit immediately
  - create and store new `flusherCtx`
  - `go flusher()`
- `inprocOffset`
  - stored in Sequencer, determined on `New()`->`actualizer()`
  - +1 and return on each Start()
- `flusher()`
  - is a goroutine
  - select
    - case `cleanupCtx`, `flusherCtx` close -> return
    - case `flushTimeout` or `toBeFlushed` queue overflow -> flush:
      - `toBeFlushed` not empty -> `ISeqStorage.WriteValues(toBeFlushed)`
      - `toBeFlushedOffset` != 0 -> `ISeqStorage.WritePlog...(toBeFlushedOffset)`
      - clear `toBeFlushed`
      - `toBeFlushedOffset` := 0
- `ISequencer.Start(wsKind, wsid)`: Handle a request, start to work with sequences
  - `cleanupCtx` closed -> panic
  - previous `Start()` call was not finished by calling `Flush()` or `Actualize()` -> panic
  - unknown `wsKind` -> panic
  - actualization is in progress -> return false
  - `toBeFlushed` is overflowed -> return false
  - `inprocOffset` += 1
    - later it will go to `toBeFlushedOffset`, zeroed on `Actualize()` only
  - return `inprocOffset` and true
- New number obtain
  - `ISequencer.Next(SeqID)` is called
    - unknown `SeqID` -> panic
    - try to get the next number (`value`) preserving order:
      - LRU cache
      - `inproc`
      - `toBeFlushed`
      - `ISeqStorage.ReadNumber`
        - read all numbers, put all number to LRU cache
    - write `value+1` to LRU cache
    - write `value+1` to `inproc`
- Event handling is finished, close sequencer session (started by `Start()`)
  - no errors -> `ISequencer.Flush()`
    - copy `inproc` -> `toBeFlushed`
    - `inprocOffset` -> `toBeFlushedOffset`
    - clear `inproc`
  - has errors -> `ISequencer.Actualize()`
    - clear `inproc`
    - `inprocOffset` = 0
    - `go actualizer()`, it reads actual data from PLog, restarts `flusher()` etc

## Test plan

- Basic usage
  - storage contains an event with PLogOffset=42
    - i.e. ActualizeSequencesFromPLog should call batcher() []SeqValue{ Key: wsid, seqID; Value: 13}, offset = 42
  - sequencer contains nothing
  - New()
  - assert Start(wsid, wsKind) return 43
  - assert Next(seqID) return 14
  - Flush()
  - wait for 500ms
  - assert ISeqStorage.ReadNumbers returns new data: offset 43 and 14 for seqID
- Actualization on error
  - New, Start, Next
  - now act like when something gone wrong on the caller side: all numbers got on Start and Next should be dropped:
  - wait for 500ms to check then that new issued number will not go to the storage
  - Actualize()
  - assert Start return the same value as on previous Start
  - assert Next return the same value as on prevois Next
- Batcher
  - few storages:
    - empty
    - 2-3 different wsids, wsKinds, values etc
  - New
  - assert Start returns the correct value for each storeg
  - asser Next returns the correct value for each storage
- Behaviour on incorrect methods calls
  - expect panics on:
    - 2nd Start()
    - Start() after cleanup
    - Next(), Flush() without Start()
