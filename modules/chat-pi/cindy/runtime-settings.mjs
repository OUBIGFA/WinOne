// Retry defaults extracted from Cindy a44ac274, buildPiSettingsJsonContent,
// Copyright 2026 XD Inc., Apache-2.0. Only the WinOne preference seam is added.
// Pi owns transient classification, backoff, cancellation and replay boundaries;
// do not add a second fetch retry loop or resubmit the original user task.
export function cindyRetrySettings(runtimeSettings={}){
 return {
  enabled:runtimeSettings.autoRetry!==false,
  maxRetries:runtimeSettings.maxRetries??6,
  baseDelayMs:2000,
  provider:{maxRetries:0},
 };
}
