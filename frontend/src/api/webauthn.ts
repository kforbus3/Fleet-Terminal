import { api } from "./client";
import type { LoginResponse } from "./auth";

// --- base64url <-> ArrayBuffer helpers (WebAuthn uses base64url everywhere) ---

function b64urlToBuf(s: string): ArrayBuffer {
  const pad = "=".repeat((4 - (s.length % 4)) % 4);
  const b64 = (s + pad).replace(/-/g, "+").replace(/_/g, "/");
  const bin = atob(b64);
  const buf = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) buf[i] = bin.charCodeAt(i);
  return buf.buffer;
}

function bufToB64url(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/* eslint-disable @typescript-eslint/no-explicit-any */
function decodeCreationOptions(pk: any): PublicKeyCredentialCreationOptions {
  pk.challenge = b64urlToBuf(pk.challenge);
  pk.user.id = b64urlToBuf(pk.user.id);
  if (pk.excludeCredentials) {
    pk.excludeCredentials = pk.excludeCredentials.map((c: any) => ({ ...c, id: b64urlToBuf(c.id) }));
  }
  return pk;
}

function decodeRequestOptions(pk: any): PublicKeyCredentialRequestOptions {
  pk.challenge = b64urlToBuf(pk.challenge);
  if (pk.allowCredentials) {
    pk.allowCredentials = pk.allowCredentials.map((c: any) => ({ ...c, id: b64urlToBuf(c.id) }));
  }
  return pk;
}

function encodeAttestation(cred: PublicKeyCredential) {
  const r = cred.response as AuthenticatorAttestationResponse;
  return {
    id: cred.id,
    rawId: bufToB64url(cred.rawId),
    type: cred.type,
    response: {
      attestationObject: bufToB64url(r.attestationObject),
      clientDataJSON: bufToB64url(r.clientDataJSON),
    },
  };
}

function encodeAssertion(cred: PublicKeyCredential) {
  const r = cred.response as AuthenticatorAssertionResponse;
  return {
    id: cred.id,
    rawId: bufToB64url(cred.rawId),
    type: cred.type,
    response: {
      authenticatorData: bufToB64url(r.authenticatorData),
      clientDataJSON: bufToB64url(r.clientDataJSON),
      signature: bufToB64url(r.signature),
      userHandle: r.userHandle ? bufToB64url(r.userHandle) : null,
    },
  };
}
/* eslint-enable @typescript-eslint/no-explicit-any */

export function webauthnSupported(): boolean {
  return typeof window !== "undefined" && !!window.PublicKeyCredential;
}

// registerPasskey runs the full registration ceremony for the signed-in user.
export async function registerPasskey(): Promise<void> {
  const { data } = await api.post("/api/v1/auth/mfa/webauthn/register/begin");
  const pk = decodeCreationOptions(data.options.publicKey);
  const cred = (await navigator.credentials.create({ publicKey: pk })) as PublicKeyCredential;
  await api.post(
    `/api/v1/auth/mfa/webauthn/register/finish?s=${encodeURIComponent(data.sessionKey)}`,
    encodeAttestation(cred),
  );
}

// authenticatePasskey runs the assertion ceremony as login step 2 and returns
// the session response.
export async function authenticatePasskey(challenge: string): Promise<LoginResponse> {
  const { data } = await api.post("/api/v1/auth/mfa/webauthn/login/begin", { challenge });
  const pk = decodeRequestOptions(data.options.publicKey);
  const cred = (await navigator.credentials.get({ publicKey: pk })) as PublicKeyCredential;
  const res = await api.post<LoginResponse>(
    `/api/v1/auth/mfa/webauthn/login/finish?s=${encodeURIComponent(data.sessionKey)}`,
    encodeAssertion(cred),
  );
  return res.data;
}
