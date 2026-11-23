"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ProviderButtons } from "@/components/provider-buttons";
import { Input } from "@/components/ui/input";
import { createAccountApi, type AccountApi, type Failure } from "@/lib/browser-api";
import type { Provider } from "@/lib/providers";

/** Seconds before "send a new code" is offered; the API enforces one code per address per minute. */
export const RESEND_AFTER_SECONDS = 60;

export function messageFor(failure: Failure, step: "email" | "code"): string {
  switch (failure.kind) {
    case "rate_limited": {
      const s = failure.retryAfterSeconds;
      const wait = s >= 120 ? `${Math.ceil(s / 60)} minutes` : `${s} second${s === 1 ? "" : "s"}`;
      return `Too many attempts. Try again in ${wait}.`;
    }
    case "invalid_request":
      return step === "email"
        ? "Enter a valid email address."
        : "Enter the 6-digit code from the email.";
    case "invalid_code":
      return "That code is wrong or has expired. Check it, or ask for a new one.";
    case "forbidden":
      return "This page is out of date. Reload it and try again.";
    default:
      return "Something went wrong on our side. Please try again in a moment.";
  }
}

export function LoginForm({
  api,
  onSignedIn,
  providers = [],
  providerError = null,
}: {
  api?: AccountApi;
  onSignedIn?: () => void;
  /** Providers the API has credentials for; none means no provider buttons. */
  providers?: Provider[];
  /** Plain-language reason a provider sign-in failed, from /login?error=. */
  providerError?: string | null;
}) {
  const router = useRouter();
  const [client] = useState(() => api ?? createAccountApi());
  const [step, setStep] = useState<"email" | "code">("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const codeRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (step === "code") codeRef.current?.focus();
  }, [step]);

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = setTimeout(() => setCooldown((c) => c - 1), 1000);
    return () => clearTimeout(timer);
  }, [cooldown]);

  async function sendCode(event?: FormEvent) {
    event?.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    const result = await client.startLogin(email);
    setBusy(false);
    if (!result.ok) {
      setError(messageFor(result, "email"));
      if (result.kind === "rate_limited") setCooldown(result.retryAfterSeconds);
      return;
    }
    setCode("");
    setStep("code");
    setCooldown(RESEND_AFTER_SECONDS);
    setNotice("Code sent. It can take a minute to arrive.");
  }

  async function submitCode(event: FormEvent) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    const result = await client.verifyLogin(email, code);
    if (!result.ok) {
      setBusy(false);
      setError(messageFor(result, "code"));
      codeRef.current?.focus();
      return;
    }
    if (onSignedIn) {
      onSignedIn();
    } else {
      router.replace("/account");
      router.refresh();
    }
  }

  function changeEmail() {
    setStep("email");
    setError(null);
    setNotice(null);
    setCode("");
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Sign in or create an account</CardTitle>
        <p className="text-muted-foreground text-sm">
          No password to remember. We email you a one-time code.
        </p>
      </CardHeader>
      <CardContent>
        {providerError ? (
          <p role="alert" className="text-bad mb-4 text-sm">
            {providerError}
          </p>
        ) : null}
        {step === "email" && providers.length > 0 ? (
          <div className="mb-4 flex flex-col gap-4">
            <ProviderButtons providers={providers} />
            <p className="text-muted-foreground text-center text-xs">or use an emailed code</p>
          </div>
        ) : null}
        {step === "email" ? (
          <form onSubmit={sendCode} className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <label htmlFor="email" className="text-sm font-medium">
                Email address
              </label>
              <Input
                id="email"
                name="email"
                type="email"
                autoComplete="email"
                inputMode="email"
                required
                maxLength={254}
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                aria-describedby={error ? "form-error" : undefined}
                aria-invalid={error ? true : undefined}
              />
            </div>
            <Feedback error={error} notice={notice} />
            <Button type="submit" disabled={busy || (cooldown > 0 && error !== null)}>
              {busy ? "Sending…" : "Send code"}
            </Button>
          </form>
        ) : (
          <form onSubmit={submitCode} className="flex flex-col gap-4">
            <p className="text-sm">
              We sent a 6-digit code to <strong className="break-all">{email.trim()}</strong>.
            </p>
            <div className="flex flex-col gap-1.5">
              <label htmlFor="code" className="text-sm font-medium">
                6-digit code
              </label>
              <Input
                id="code"
                name="code"
                ref={codeRef}
                autoComplete="one-time-code"
                inputMode="numeric"
                pattern="[0-9]{6}"
                maxLength={6}
                required
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
                aria-describedby={error ? "form-error" : undefined}
                aria-invalid={error ? true : undefined}
                className="text-center font-mono text-lg tracking-[0.4em]"
              />
            </div>
            <Feedback error={error} notice={notice} />
            <Button type="submit" disabled={busy || code.length !== 6}>
              {busy ? "Checking…" : "Sign in"}
            </Button>
            <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
              <button type="button" onClick={changeEmail} className="underline">
                Use a different email
              </button>
              <button
                type="button"
                onClick={() => void sendCode()}
                disabled={busy || cooldown > 0}
                className="underline disabled:no-underline disabled:opacity-60"
              >
                {cooldown > 0 ? `Send a new code in ${cooldown}s` : "Send a new code"}
              </button>
            </div>
          </form>
        )}
      </CardContent>
    </Card>
  );
}

function Feedback({ error, notice }: { error: string | null; notice: string | null }) {
  return (
    <>
      {error ? (
        <p id="form-error" role="alert" className="text-bad text-sm">
          {error}
        </p>
      ) : null}
      {notice ? (
        <p role="status" className="text-muted-foreground text-sm">
          {notice}
        </p>
      ) : null}
    </>
  );
}
