import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AccountApi, Failure } from "@/lib/browser-api";
import { me } from "@/test/account-fixtures";
import { LoginForm, RESEND_AFTER_SECONDS, messageFor } from "./login-form";

const router = vi.hoisted(() => ({ replace: vi.fn(), refresh: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => router }));

function fakeApi(overrides: Partial<AccountApi> = {}): AccountApi {
  return {
    startLogin: vi.fn(async () => ({ ok: true as const })),
    verifyLogin: vi.fn(async () => ({ ok: true as const, me: me() })),
    logout: vi.fn(async () => ({ ok: true as const })),
    rename: vi.fn(),
    listSessions: vi.fn(),
    revokeSession: vi.fn(),
    listIdentities: vi.fn(),
    startConnect: vi.fn(),
    disconnect: vi.fn(),
    ...overrides,
  };
}

const fail = (f: Failure) => async () => f;

function typeInto(label: RegExp, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
}

async function reachCodeStep(api: AccountApi, onSignedIn = vi.fn()) {
  render(<LoginForm api={api} onSignedIn={onSignedIn} />);
  typeInto(/email address/i, "ada@example.com");
  fireEvent.click(screen.getByRole("button", { name: /send code/i }));
  await screen.findByLabelText(/6-digit code/i);
  return onSignedIn;
}

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe("LoginForm: email step", () => {
  it("asks for an email with a proper label and autocomplete", () => {
    render(<LoginForm api={fakeApi()} />);
    const input = screen.getByLabelText(/email address/i);
    expect(input).toHaveAttribute("type", "email");
    expect(input).toHaveAttribute("autocomplete", "email");
    expect(input).toBeRequired();
  });

  it("sends the code and moves to the code step with focus on the code field", async () => {
    const api = fakeApi();
    await reachCodeStep(api);
    expect(api.startLogin).toHaveBeenCalledWith("ada@example.com");
    expect(screen.getByText("ada@example.com")).toBeInTheDocument();
    expect(screen.getByLabelText(/6-digit code/i)).toHaveFocus();
    expect(screen.getByLabelText(/6-digit code/i)).toHaveAttribute("autocomplete", "one-time-code");
    expect(screen.getByRole("status")).toHaveTextContent(/code sent/i);
  });

  it("shows a validation message for an address the server refuses", async () => {
    render(
      <LoginForm api={fakeApi({ startLogin: fail({ ok: false, kind: "invalid_request" }) })} />,
    );
    typeInto(/email address/i, "nope@x");
    fireEvent.click(screen.getByRole("button", { name: /send code/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/valid email address/i);
    expect(screen.getByLabelText(/email address/i)).toHaveAttribute("aria-invalid", "true");
    expect(screen.queryByLabelText(/6-digit code/i)).not.toBeInTheDocument();
  });

  it("explains a rate limit with the wait and blocks a retry until it passes", async () => {
    render(
      <LoginForm
        api={fakeApi({
          startLogin: fail({ ok: false, kind: "rate_limited", retryAfterSeconds: 42 }),
        })}
      />,
    );
    typeInto(/email address/i, "ada@example.com");
    fireEvent.click(screen.getByRole("button", { name: /send code/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Try again in 42 seconds");
    expect(screen.getByRole("button", { name: /send code/i })).toBeDisabled();
  });

  it("shows a generic message when the service is down", async () => {
    render(<LoginForm api={fakeApi({ startLogin: fail({ ok: false, kind: "unavailable" }) })} />);
    typeInto(/email address/i, "ada@example.com");
    fireEvent.click(screen.getByRole("button", { name: /send code/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/something went wrong/i);
  });

  it("ignores a second submit while the first is running", async () => {
    let release: () => void = () => {};
    const startLogin = vi.fn(
      () => new Promise<{ ok: true }>((resolve) => (release = () => resolve({ ok: true }))),
    );
    render(<LoginForm api={fakeApi({ startLogin })} />);
    typeInto(/email address/i, "ada@example.com");
    const form = screen.getByRole("button", { name: /send code/i }).closest("form")!;
    fireEvent.submit(form);
    fireEvent.submit(form);
    expect(startLogin).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: /sending/i })).toBeDisabled();
    await act(async () => release());
  });
});

describe("LoginForm: code step", () => {
  it("keeps only digits, at most six", async () => {
    await reachCodeStep(fakeApi());
    const input = screen.getByLabelText(/6-digit code/i);
    fireEvent.change(input, { target: { value: "12a3 4-5678" } });
    expect(input).toHaveValue("123456");
  });

  it("enables Sign in only for six digits", async () => {
    await reachCodeStep(fakeApi());
    const button = screen.getByRole("button", { name: /^sign in$/i });
    expect(button).toBeDisabled();
    typeInto(/6-digit code/i, "12345");
    expect(button).toBeDisabled();
    typeInto(/6-digit code/i, "123456");
    expect(button).toBeEnabled();
  });

  it("signs in with the right code and tells the page to move on", async () => {
    const api = fakeApi();
    const onSignedIn = await reachCodeStep(api);
    typeInto(/6-digit code/i, "013842");
    fireEvent.click(screen.getByRole("button", { name: /^sign in$/i }));
    await waitFor(() => expect(onSignedIn).toHaveBeenCalledOnce());
    expect(api.verifyLogin).toHaveBeenCalledWith("ada@example.com", "013842");
  });

  it("uses the router when no callback is given", async () => {
    const api = fakeApi();
    render(<LoginForm api={api} />);
    typeInto(/email address/i, "ada@example.com");
    fireEvent.click(screen.getByRole("button", { name: /send code/i }));
    await screen.findByLabelText(/6-digit code/i);
    typeInto(/6-digit code/i, "013842");
    fireEvent.click(screen.getByRole("button", { name: /^sign in$/i }));
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/account"));
    expect(router.refresh).toHaveBeenCalled();
  });

  it("shows one message for a wrong, expired or used code and keeps the field", async () => {
    const api = fakeApi({ verifyLogin: fail({ ok: false, kind: "invalid_code" }) });
    const onSignedIn = await reachCodeStep(api);
    typeInto(/6-digit code/i, "000000");
    fireEvent.click(screen.getByRole("button", { name: /^sign in$/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/wrong or has expired/i);
    expect(onSignedIn).not.toHaveBeenCalled();
    expect(screen.getByLabelText(/6-digit code/i)).toHaveFocus();
    expect(screen.getByLabelText(/6-digit code/i)).toHaveAttribute("aria-invalid", "true");
  });

  it("ignores a second submit of the code while the first is being checked", async () => {
    let release: () => void = () => {};
    const verifyLogin = vi.fn(
      () =>
        new Promise<{ ok: true; me: ReturnType<typeof me> }>(
          (resolve) => (release = () => resolve({ ok: true, me: me() })),
        ),
    );
    const onSignedIn = await reachCodeStep(fakeApi({ verifyLogin }));
    typeInto(/6-digit code/i, "013842");
    const form = screen.getByRole("button", { name: /^sign in$/i }).closest("form")!;
    fireEvent.submit(form);
    fireEvent.submit(form);
    expect(verifyLogin).toHaveBeenCalledTimes(1);
    await act(async () => release());
    expect(onSignedIn).toHaveBeenCalledOnce();
  });

  it("lets the person go back and use another address", async () => {
    await reachCodeStep(fakeApi());
    fireEvent.click(screen.getByRole("button", { name: /use a different email/i }));
    expect(screen.getByLabelText(/email address/i)).toHaveValue("ada@example.com");
    expect(screen.queryByLabelText(/6-digit code/i)).not.toBeInTheDocument();
  });

  it("offers a new code only after the cooldown", async () => {
    vi.useFakeTimers();
    const api = fakeApi();
    render(<LoginForm api={api} onSignedIn={vi.fn()} />);
    typeInto(/email address/i, "ada@example.com");
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /send code/i }));
    });
    const resend = () => screen.getByRole("button", { name: /send a new code/i });
    expect(resend()).toBeDisabled();
    expect(resend()).toHaveTextContent(`in ${RESEND_AFTER_SECONDS}s`);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(resend()).toHaveTextContent(`in ${RESEND_AFTER_SECONDS - 1}s`);

    // The countdown re-renders every second, so tick one second at a time.
    for (let i = 0; i < RESEND_AFTER_SECONDS - 1; i++) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
    }
    expect(resend()).toBeEnabled();
    expect(resend()).toHaveTextContent(/^send a new code$/i);

    await act(async () => {
      fireEvent.click(resend());
    });
    expect(api.startLogin).toHaveBeenCalledTimes(2);
    expect(resend()).toBeDisabled();
  });
});

describe("messageFor", () => {
  it.each([
    [{ ok: false, kind: "rate_limited", retryAfterSeconds: 1 }, "email", "1 second."],
    [{ ok: false, kind: "rate_limited", retryAfterSeconds: 90 }, "email", "90 seconds."],
    [{ ok: false, kind: "rate_limited", retryAfterSeconds: 3000 }, "email", "50 minutes."],
    [{ ok: false, kind: "invalid_request" }, "code", "6-digit code"],
    [{ ok: false, kind: "forbidden" }, "email", "Reload"],
    [{ ok: false, kind: "unauthenticated" }, "email", "went wrong"],
    [{ ok: false, kind: "not_found" }, "email", "went wrong"],
  ] as [Failure, "email" | "code", string][])("%j on the %s step contains %s", (f, step, text) => {
    expect(messageFor(f, step)).toContain(text);
  });
});

describe("LoginForm: provider sign-in", () => {
  it("shows no provider buttons unless a provider is configured", () => {
    render(<LoginForm api={fakeApi()} />);
    expect(screen.queryByRole("link", { name: /continue with/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/or use an emailed code/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText(/email address/i)).toBeInTheDocument();
  });

  it("shows one plain link per configured provider, to the bridge's start route", () => {
    render(<LoginForm api={fakeApi()} providers={["google", "github"]} />);
    expect(screen.getByRole("link", { name: "Continue with Google" })).toHaveAttribute(
      "href",
      "/api/v1/auth/google/start",
    );
    expect(screen.getByRole("link", { name: "Continue with GitHub" })).toHaveAttribute(
      "href",
      "/api/v1/auth/github/start",
    );
  });

  it("shows only the providers it is given", () => {
    render(<LoginForm api={fakeApi()} providers={["github"]} />);
    expect(screen.queryByRole("link", { name: /google/i })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: /github/i })).toBeInTheDocument();
  });

  it("keeps the email form beside the buttons, and hides the buttons on the code step", async () => {
    const api = fakeApi();
    render(<LoginForm api={api} providers={["google"]} />);
    typeInto(/email address/i, "ada@example.com");
    fireEvent.click(screen.getByRole("button", { name: /send code/i }));
    await screen.findByLabelText(/6-digit code/i);
    expect(screen.queryByRole("link", { name: /continue with/i })).not.toBeInTheDocument();
  });

  it("announces why a provider sign-in failed, as text", () => {
    render(
      <LoginForm
        api={fakeApi()}
        providerError="Sign-in was cancelled. <b>x</b>"
        providers={["google"]}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Sign-in was cancelled. <b>x</b>");
    expect(document.querySelector("b")).toBeNull();
  });

  it("shows no alert when nothing failed", () => {
    render(<LoginForm api={fakeApi()} providers={["google"]} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
