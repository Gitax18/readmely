"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { Identity, Me, Session, User } from "@/lib/account-schema";
import { createAccountApi, type AccountApi, type Failure } from "@/lib/browser-api";
import { PROVIDER_LABELS, PROVIDERS, type Provider } from "@/lib/providers";

const dateFormat = new Intl.DateTimeFormat("en", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "UTC",
});

/** Fixed time zone so the server render and the browser render agree (no hydration mismatch). */
export function formatWhen(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "unknown" : `${dateFormat.format(d)} UTC`;
}

function failureText(f: Failure): string {
  switch (f.kind) {
    case "invalid_request":
      return "A name needs 1 to 80 characters and no control characters.";
    case "forbidden":
      return "This page is out of date. Reload it and try again.";
    case "rate_limited":
      return "Too many requests. Try again shortly.";
    case "conflict":
      return "That account is already connected. Reload the page to see it.";
    default:
      return "Something went wrong on our side. Please try again in a moment.";
  }
}

export function AccountView({
  me,
  initialSessions,
  initialIdentities = [],
  providers = [],
  notice = null,
  api,
  onSignedOut,
  onNavigate,
}: {
  me: Me;
  initialSessions: Session[];
  /** The provider accounts already connected. */
  initialIdentities?: Identity[];
  /** Providers the API has credentials for: only these offer "Connect". */
  providers?: Provider[];
  /** Why a connect attempt failed, from /account?error=, in plain words. */
  notice?: string | null;
  api?: AccountApi;
  onSignedOut?: () => void;
  /** Leaves for the provider; replaceable in tests. */
  onNavigate?: (url: string) => void;
}) {
  const router = useRouter();
  const [client] = useState(() => api ?? createAccountApi());
  const [user, setUser] = useState<User>(me.user);
  const [name, setName] = useState(me.user.display_name);
  const [sessions, setSessions] = useState<Session[]>(initialSessions);
  const [nameMessage, setNameMessage] = useState<{ text: string; isError: boolean } | null>(null);
  const [sessionError, setSessionError] = useState<string | null>(null);
  const [identities, setIdentities] = useState<Identity[]>(initialIdentities);
  const [confirming, setConfirming] = useState<Provider | null>(null);
  const [identityError, setIdentityError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  function leave() {
    if (onSignedOut) {
      onSignedOut();
    } else {
      router.replace("/login");
      router.refresh();
    }
  }

  // A 401 anywhere means the session ended (revoked elsewhere, expired): back to sign-in.
  function handleFailure(f: Failure, show: (text: string) => void) {
    if (f.kind === "unauthenticated") {
      leave();
      return;
    }
    show(failureText(f));
  }

  async function saveName(event: FormEvent) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setNameMessage(null);
    const result = await client.rename(me.csrf_token, name);
    setBusy(false);
    if (result.ok) {
      setUser(result.user);
      setName(result.user.display_name);
      setNameMessage({ text: "Saved.", isError: false });
    } else {
      handleFailure(result, (text) => setNameMessage({ text, isError: true }));
    }
  }

  async function signOut() {
    if (busy) return;
    setBusy(true);
    const result = await client.logout(me.csrf_token);
    if (result.ok || result.kind === "unauthenticated") {
      leave();
      return;
    }
    setBusy(false);
    setSessionError(failureText(result));
  }

  async function revoke(id: string) {
    if (busy) return;
    setBusy(true);
    setSessionError(null);
    const result = await client.revokeSession(me.csrf_token, id);
    if (!result.ok && result.kind !== "not_found") {
      setBusy(false);
      handleFailure(result, setSessionError);
      return;
    }
    const refreshed = await client.listSessions();
    setBusy(false);
    if (refreshed.ok) setSessions(refreshed.sessions);
    else handleFailure(refreshed, setSessionError);
  }

  async function connect(provider: Provider) {
    if (busy) return;
    setBusy(true);
    setIdentityError(null);
    const result = await client.startConnect(me.csrf_token, provider);
    if (!result.ok) {
      setBusy(false);
      handleFailure(result, setIdentityError);
      return;
    }
    // Leaves the page; busy stays on so nothing else is clicked on the way out.
    (onNavigate ?? ((url: string) => window.location.assign(url)))(result.url);
  }

  async function disconnect(provider: Provider) {
    if (busy) return;
    setBusy(true);
    setIdentityError(null);
    const result = await client.disconnect(me.csrf_token, provider);
    if (!result.ok && result.kind !== "not_found") {
      setBusy(false);
      setConfirming(null);
      handleFailure(result, setIdentityError);
      return;
    }
    const refreshed = await client.listIdentities();
    setBusy(false);
    setConfirming(null);
    if (refreshed.ok) setIdentities(refreshed.identities);
    else handleFailure(refreshed, setIdentityError);
  }

  // A provider is listed when it can be connected, or when an account is connected to it already
  // (so it can still be removed after the provider's credentials are taken away).
  const shown = PROVIDERS.filter(
    (p) => providers.includes(p) || identities.some((i) => i.provider === p),
  );

  return (
    <div className="flex flex-col gap-6">
      {notice ? (
        <p role="alert" className="text-bad rounded-md border border-bad px-4 py-3 text-sm">
          {notice}
        </p>
      ) : null}
      <Card>
        <CardHeader>
          <CardTitle>Your account</CardTitle>
          <p className="text-muted-foreground text-sm">
            Signed in as <strong className="text-foreground break-all">{user.email}</strong>
          </p>
        </CardHeader>
        <CardContent>
          <form onSubmit={saveName} className="flex flex-col gap-3">
            <label htmlFor="display-name" className="text-sm font-medium">
              Display name
            </label>
            <div className="flex flex-col gap-2 sm:flex-row">
              <Input
                id="display-name"
                name="display_name"
                autoComplete="name"
                maxLength={80}
                value={name}
                onChange={(e) => setName(e.target.value)}
                aria-describedby="name-message"
              />
              <Button
                type="submit"
                disabled={busy || name.trim() === "" || name.trim() === user.display_name}
              >
                Save
              </Button>
            </div>
            <p
              id="name-message"
              role={nameMessage?.isError ? "alert" : "status"}
              className={
                nameMessage?.isError ? "text-bad text-sm" : "text-muted-foreground text-sm"
              }
            >
              {nameMessage?.text ?? ""}
            </p>
          </form>
        </CardContent>
      </Card>

      {shown.length > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>Connected accounts</CardTitle>
            <p className="text-muted-foreground text-sm">
              Sign in with a connected account instead of waiting for a code. An emailed code always
              works, so disconnecting never locks you out.
            </p>
          </CardHeader>
          <CardContent>
            <ul className="flex flex-col divide-y divide-border" aria-label="Connected accounts">
              {shown.map((p) => {
                const identity = identities.find((i) => i.provider === p);
                const label = PROVIDER_LABELS[p];
                return (
                  <li key={p} className="flex flex-col gap-3 py-3">
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div className="flex flex-col gap-0.5">
                        <span className="text-sm font-medium">{label}</span>
                        <span className="text-muted-foreground text-xs">
                          {identity ? (
                            <>
                              Connected as <span className="break-all">{identity.email}</span> on{" "}
                              <time dateTime={identity.created_at}>
                                {formatWhen(identity.created_at)}
                              </time>
                            </>
                          ) : (
                            "Not connected"
                          )}
                        </span>
                      </div>
                      {identity ? (
                        confirming === p ? null : (
                          <Button
                            variant="secondary"
                            onClick={() => setConfirming(p)}
                            disabled={busy}
                            aria-label={`Disconnect ${label}`}
                          >
                            Disconnect
                          </Button>
                        )
                      ) : (
                        <Button
                          onClick={() => void connect(p)}
                          disabled={busy}
                          aria-label={`Connect ${label}`}
                        >
                          Connect
                        </Button>
                      )}
                    </div>
                    {identity && confirming === p ? (
                      <div
                        role="group"
                        aria-label={`Confirm disconnecting ${label}`}
                        className="flex flex-col gap-3 rounded-md border border-border p-3"
                      >
                        <p className="text-sm">
                          Disconnect {label}? You can still sign in with an emailed code, and we
                          will email you about the change.
                        </p>
                        <div className="flex gap-2">
                          <Button
                            variant="danger"
                            onClick={() => void disconnect(p)}
                            disabled={busy}
                          >
                            Yes, disconnect {label}
                          </Button>
                          <Button
                            variant="secondary"
                            onClick={() => setConfirming(null)}
                            disabled={busy}
                          >
                            Keep it
                          </Button>
                        </div>
                      </div>
                    ) : null}
                  </li>
                );
              })}
            </ul>
            {identityError ? (
              <p role="alert" className="text-bad mt-3 text-sm">
                {identityError}
              </p>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>Where you are signed in</CardTitle>
          <p className="text-muted-foreground text-sm">
            Sign out any device you do not recognise. Sessions end after 7 days without use, or 30
            days in all.
          </p>
        </CardHeader>
        <CardContent>
          <ul className="flex flex-col divide-y divide-border" aria-label="Active sessions">
            {sessions.map((s) => (
              <li key={s.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
                <div className="flex flex-col gap-0.5">
                  <span className="flex items-center gap-2 text-sm font-medium">
                    {s.device || "Unknown device"}
                    {s.current ? <Badge>This device</Badge> : null}
                  </span>
                  <span className="text-muted-foreground text-xs">
                    Signed in <time dateTime={s.created_at}>{formatWhen(s.created_at)}</time>, last
                    used <time dateTime={s.last_seen_at}>{formatWhen(s.last_seen_at)}</time>
                  </span>
                </div>
                {s.current ? null : (
                  <Button
                    variant="secondary"
                    onClick={() => void revoke(s.id)}
                    disabled={busy}
                    aria-label={`Sign out ${s.device || "unknown device"} (signed in ${formatWhen(s.created_at)})`}
                  >
                    Sign out
                  </Button>
                )}
              </li>
            ))}
          </ul>
          {sessionError ? (
            <p role="alert" className="text-bad mt-3 text-sm">
              {sessionError}
            </p>
          ) : null}
          <div className="mt-4">
            <Button variant="danger" onClick={() => void signOut()} disabled={busy}>
              Sign out of this device
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
