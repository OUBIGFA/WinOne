package croc

import "math"

// Only measured, JSON-safe values cross the host boundary. A just-created or
// zero-size upstream bar can have NaN estimates; unknown speed stays absent.
type WinOneFileProgress struct {
 Max int64
 CurrentNum int64
 Description string
 KBsPerSecond *float64
}

// WinOneTransferProgress is read-only host integration. Never read exported
// mutable Client fields from a polling UI: the upstream lifecycle/bar locks
// are the synchronization boundary while Send or Receive is running.
type WinOneTransferProgress struct {
 Secured bool
 MetadataReady bool
 Complete bool
 Progress *WinOneFileProgress
}

func (c *Client) WinOneProgress() WinOneTransferProgress {
 lifecycle:=c.lifecycleSnapshot()
 result:=WinOneTransferProgress{Secured:lifecycle.ChannelSecured, MetadataReady:lifecycle.FileInfoTransferred, Complete:lifecycle.Successful}
 c.barMu.RLock()
 bar:=c.bar
 c.barMu.RUnlock()
 if bar!=nil {
  state:=bar.State()
  result.Progress=&WinOneFileProgress{Max:state.Max,CurrentNum:state.CurrentNum,Description:state.Description}
  if !math.IsNaN(state.KBsPerSecond)&&!math.IsInf(state.KBsPerSecond,0) { result.Progress.KBsPerSecond=&state.KBsPerSecond }
 }
 return result
}
