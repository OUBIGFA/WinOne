

export class CredentialModeSwitchBusyError extends Error {
  readonly sessionIds: string[];

  constructor(
    sessionIds: string[],
    message = `Cannot switch credential mode while local session(s) are busy: ${sessionIds.join(', ')}`,
  ) {
    super(message);
    this.name = 'CredentialModeSwitchBusyError';
    this.sessionIds = sessionIds;
  }
}


export function isCredentialModeSwitchBusyError(
  err: unknown,
): err is CredentialModeSwitchBusyError {
  return err instanceof CredentialModeSwitchBusyError;
}
export {  };
