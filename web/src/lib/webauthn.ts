type JSONOptions = { publicKey: Record<string, unknown> };

export function passkeysSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.PublicKeyCredential !== "undefined" &&
    typeof PublicKeyCredential.parseRequestOptionsFromJSON === "function"
  );
}

export async function getAssertion(options: JSONOptions, signal?: AbortSignal): Promise<unknown> {
  const publicKey = PublicKeyCredential.parseRequestOptionsFromJSON(options.publicKey as unknown as PublicKeyCredentialRequestOptionsJSON);
  const credential = (await navigator.credentials.get({ publicKey, signal })) as PublicKeyCredential | null;
  if (!credential) throw new DOMException("No passkey was selected.", "NotAllowedError");
  return credential.toJSON();
}

export async function createCredential(options: JSONOptions, signal?: AbortSignal): Promise<unknown> {
  const publicKey = PublicKeyCredential.parseCreationOptionsFromJSON(options.publicKey as unknown as PublicKeyCredentialCreationOptionsJSON);
  const credential = (await navigator.credentials.create({ publicKey, signal })) as PublicKeyCredential | null;
  if (!credential) throw new DOMException("No passkey was created.", "NotAllowedError");
  return credential.toJSON();
}

export function describeWebAuthnError(error: unknown): string {
  if (error instanceof DOMException) {
    if (error.name === "NotAllowedError") return "The passkey prompt was closed or timed out. Try again when you're ready.";
    if (error.name === "InvalidStateError") return "This device already has a passkey for your account. Use it to sign in instead.";
    if (error.name === "SecurityError") return "Passkeys need a secure address. Open Halo over https or on localhost.";
  }
  return error instanceof Error ? error.message : "Something went wrong with the passkey. Try again.";
}
