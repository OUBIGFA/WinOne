

export class RemoteResourceRegistryError extends Error {
  constructor(
    readonly code: RemoteResourceRegistryErrorCode,
    message: string,
  ) {
    super(message);
    this.name = 'RemoteResourceRegistryError';
  }
}
export {  };
