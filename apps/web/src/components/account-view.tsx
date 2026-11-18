"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { Me, Session, User } from "@/lib/account-schema";
import { createAccountApi, type AccountApi, type Failure } from "@/lib/browser-api";

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
    default:
      return "Something went wrong on our side. Please try again in a moment.";
  }
}

export function AccountView({
  me,
  initialSessions,
  api,
  onSignedOut,
}: {
  me: Me;
  initialSessions: Session[];
  api?: AccountApi;
  onSignedOut?: () => void;
}) {
  const router = useRouter();
  const [client] = useState(() => api ?? createAccountApi());
  const [user, setUser] = useState<User>(me.user);
  const [name, setName] = useState(me.user.display_name);
  const [sessions, setSessions] = useState<Session[]>(initialSessions);
  const [nameMessage, setNameMessage] = useState<{ text: string; isError: boolean } | null>(null);
  const [sessionError, setSessionError] = useState<string | null>(null);
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

  return (
    <div className="flex flex-col gap-6">
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
