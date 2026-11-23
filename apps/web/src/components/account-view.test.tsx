import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AccountApi, Failure } from "@/lib/browser-api";
import { CSRF, identity, me, session } from "@/test/account-fixtures";
import { AccountView, formatWhen } from "./account-view";

const router = vi.hoisted(() => ({ replace: vi.fn(), refresh: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => router }));

const OTHER = session({
  id: "01900000-0000-7000-8000-000000000402",
  device: "Chrome on Windows",
  current: false,
});

function fakeApi(overrides: Partial<AccountApi> = {}): AccountApi {
  return {
    startLogin: vi.fn(),
    verifyLogin: vi.fn(),
    logout: vi.fn(async () => ({ ok: true as const })),
    rename: vi.fn(async (_c: string, name: string) => ({
      ok: true as const,
      user: me({ display_name: name }).user,
    })),
    listSessions: vi.fn(async () => ({ ok: true as const, sessions: [session()] })),
    revokeOtherSessions: vi.fn(),
    startDeletion: vi.fn(),
    confirmDeletion: vi.fn(),
    revokeSession: vi.fn(async () => ({ ok: true as const })),
    listIdentities: vi.fn(async () => ({ ok: true as const, identities: [] })),
    startConnect: vi.fn(),
    disconnect: vi.fn(async () => ({ ok: true as const })),
    ...overrides,
  };
}

const fail = (f: Failure) => async () => f;

afterEach(() => vi.clearAllMocks());

describe("AccountView", () => {
  it("shows who is signed in, with the address as text (never as markup)", () => {
    render(
      <AccountView
        me={me({ email: "<img src=x onerror=alert(1)>@example.com" })}
        initialSessions={[session()]}
        api={fakeApi()}
      />,
    );
    expect(screen.getByText("<img src=x onerror=alert(1)>@example.com")).toBeInTheDocument();
    expect(document.querySelector("img")).toBeNull();
  });

  it("lists sessions, marks this device, and offers sign-out only for the others", () => {
    render(<AccountView me={me()} initialSessions={[session(), OTHER]} api={fakeApi()} />);
    const items = within(screen.getByRole("list", { name: /active sessions/i })).getAllByRole(
      "listitem",
    );
    expect(items).toHaveLength(2);
    expect(within(items[0]!).getByText("This device")).toBeInTheDocument();
    expect(within(items[0]!).queryByRole("button")).not.toBeInTheDocument();
    expect(
      within(items[1]!).getByRole("button", { name: /sign out chrome on windows/i }),
    ).toBeInTheDocument();
  });

  it("labels a session with no device as unknown", () => {
    render(<AccountView me={me()} initialSessions={[session({ device: "" })]} api={fakeApi()} />);
    expect(screen.getByText("Unknown device")).toBeInTheDocument();
  });

  it("saves a display name using this session's CSRF token", async () => {
    const api = fakeApi();
    render(<AccountView me={me()} initialSessions={[session()]} api={api} />);
    const save = screen.getByRole("button", { name: /^save$/i });
    expect(save).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/display name/i), {
      target: { value: "  Ada Lovelace " },
    });
    expect(save).toBeEnabled();
    fireEvent.click(save);
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Saved."));
    expect(api.rename).toHaveBeenCalledWith(CSRF, "  Ada Lovelace ");
  });

  it("shows a rule message when the name is refused", async () => {
    const api = fakeApi({ rename: fail({ ok: false, kind: "invalid_request" }) });
    render(<AccountView me={me()} initialSessions={[session()]} api={api} />);
    fireEvent.change(screen.getByLabelText(/display name/i), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/1 to 80 characters/);
  });

  it("revokes another session and reloads the list", async () => {
    const api = fakeApi();
    render(<AccountView me={me()} initialSessions={[session(), OTHER]} api={api} />);
    fireEvent.click(screen.getByRole("button", { name: /sign out chrome on windows/i }));
    await waitFor(() => expect(screen.queryByText("Chrome on Windows")).not.toBeInTheDocument());
    expect(api.revokeSession).toHaveBeenCalledWith(CSRF, OTHER.id);
    expect(api.listSessions).toHaveBeenCalled();
  });

  it("refreshes the list even when the session was already gone", async () => {
    const api = fakeApi({ revokeSession: fail({ ok: false, kind: "not_found" }) });
    render(<AccountView me={me()} initialSessions={[session(), OTHER]} api={api} />);
    fireEvent.click(screen.getByRole("button", { name: /sign out chrome on windows/i }));
    await waitFor(() => expect(screen.queryByText("Chrome on Windows")).not.toBeInTheDocument());
  });

  it("shows an error and keeps the list when revoking fails", async () => {
    const api = fakeApi({ revokeSession: fail({ ok: false, kind: "unavailable" }) });
    render(<AccountView me={me()} initialSessions={[session(), OTHER]} api={api} />);
    fireEvent.click(screen.getByRole("button", { name: /sign out chrome on windows/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/something went wrong/i);
    expect(screen.getByText("Chrome on Windows")).toBeInTheDocument();
  });

  it("signs out and leaves", async () => {
    const api = fakeApi();
    const onSignedOut = vi.fn();
    render(
      <AccountView me={me()} initialSessions={[session()]} api={api} onSignedOut={onSignedOut} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /sign out of this device/i }));
    await waitFor(() => expect(onSignedOut).toHaveBeenCalledOnce());
    expect(api.logout).toHaveBeenCalledWith(CSRF);
  });

  it("uses the router to leave when no callback is given", async () => {
    render(<AccountView me={me()} initialSessions={[session()]} api={fakeApi()} />);
    fireEvent.click(screen.getByRole("button", { name: /sign out of this device/i }));
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/login"));
    expect(router.refresh).toHaveBeenCalled();
  });

  it("leaves when the session has already ended (401), anywhere", async () => {
    const onSignedOut = vi.fn();
    const api = fakeApi({ rename: fail({ ok: false, kind: "unauthenticated" }) });
    render(
      <AccountView me={me()} initialSessions={[session()]} api={api} onSignedOut={onSignedOut} />,
    );
    fireEvent.change(screen.getByLabelText(/display name/i), { target: { value: "Ada" } });
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    await waitFor(() => expect(onSignedOut).toHaveBeenCalledOnce());
  });

  it("stays and explains when sign-out fails for another reason", async () => {
    const onSignedOut = vi.fn();
    const api = fakeApi({ logout: fail({ ok: false, kind: "forbidden" }) });
    render(
      <AccountView me={me()} initialSessions={[session()]} api={api} onSignedOut={onSignedOut} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /sign out of this device/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/out of date/i);
    expect(onSignedOut).not.toHaveBeenCalled();
  });

  it("leaves when reloading the list says the session ended", async () => {
    const onSignedOut = vi.fn();
    const api = fakeApi({ listSessions: fail({ ok: false, kind: "unauthenticated" }) });
    render(
      <AccountView
        me={me()}
        initialSessions={[session(), OTHER]}
        api={api}
        onSignedOut={onSignedOut}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /sign out chrome on windows/i }));
    await waitFor(() => expect(onSignedOut).toHaveBeenCalledOnce());
  });

  it("shows rate limiting in plain words", async () => {
    const api = fakeApi({
      rename: fail({ ok: false, kind: "rate_limited", retryAfterSeconds: 5 }),
    });
    render(<AccountView me={me()} initialSessions={[session()]} api={api} />);
    fireEvent.change(screen.getByLabelText(/display name/i), { target: { value: "Ada" } });
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/too many requests/i);
  });
});

describe("formatWhen", () => {
  it("is fixed to UTC so the server and the browser render the same text", () => {
    expect(formatWhen("2026-11-18T09:30:00Z")).toBe("Nov 18, 2026, 9:30 AM UTC");
  });
  it("does not throw on junk", () => {
    expect(formatWhen("not a date")).toBe("unknown");
  });
});

const GOOGLE_URL = "https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y";

describe("AccountView: connected accounts", () => {
  const renderWith = (props: Partial<React.ComponentProps<typeof AccountView>>, api = fakeApi()) =>
    render(<AccountView me={me()} initialSessions={[session()]} api={api} {...props} />);

  it("is not shown when no provider is configured and none is connected", () => {
    renderWith({});
    expect(screen.queryByText("Connected accounts")).not.toBeInTheDocument();
  });

  it("offers Connect for a configured provider that is not connected", () => {
    renderWith({
      providers: ["google", "github"],
      initialIdentities: [identity({ provider: "github", email: "octo@example.com" })],
    });
    expect(screen.getByRole("button", { name: "Connect Google" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect GitHub" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Disconnect GitHub" })).toBeInTheDocument();
    expect(screen.getByText("octo@example.com")).toBeInTheDocument();
  });

  it("still lists a connected provider that is no longer configured, so it can be removed", () => {
    renderWith({ providers: [], initialIdentities: [identity({ provider: "google" })] });
    expect(screen.getByRole("button", { name: "Disconnect Google" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^connect google$/i })).not.toBeInTheDocument();
  });

  it("shows the provider's address as text, never as markup", () => {
    renderWith({
      initialIdentities: [identity({ email: "<img src=x onerror=alert(1)>@example.com" })],
    });
    expect(screen.getByText("<img src=x onerror=alert(1)>@example.com")).toBeInTheDocument();
    expect(document.querySelector("img")).toBeNull();
  });

  it("connects: posts with the CSRF token and leaves for the provider's URL", async () => {
    const onNavigate = vi.fn();
    const startConnect = vi.fn(async () => ({ ok: true as const, url: GOOGLE_URL }));
    renderWith({ providers: ["google"], onNavigate }, fakeApi({ startConnect }));
    fireEvent.click(screen.getByRole("button", { name: "Connect Google" }));
    await waitFor(() => expect(onNavigate).toHaveBeenCalledWith(GOOGLE_URL));
    expect(startConnect).toHaveBeenCalledWith(CSRF, "google");
    // Nothing else can be clicked while leaving.
    expect(screen.getByRole("button", { name: "Connect Google" })).toBeDisabled();
  });

  it.each([
    ["conflict", { ok: false, kind: "conflict" } as const, /already connected/i],
    ["forbidden", { ok: false, kind: "forbidden" } as const, /out of date/i],
    ["unavailable", { ok: false, kind: "unavailable" } as const, /something went wrong/i],
  ])("shows a connect failure (%s) and stays on the page", async (_n, failure, text) => {
    const onNavigate = vi.fn();
    renderWith({ providers: ["google"], onNavigate }, fakeApi({ startConnect: fail(failure) }));
    fireEvent.click(screen.getByRole("button", { name: "Connect Google" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(text);
    expect(onNavigate).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Connect Google" })).toBeEnabled();
  });

  it("an ended session during connect goes back to sign-in", async () => {
    const onSignedOut = vi.fn();
    renderWith(
      { providers: ["google"], onSignedOut },
      fakeApi({ startConnect: fail({ ok: false, kind: "unauthenticated" }) }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Connect Google" }));
    await waitFor(() => expect(onSignedOut).toHaveBeenCalled());
  });

  it("asks before disconnecting, and Keep it changes nothing", () => {
    const disconnect = vi.fn(async () => ({ ok: true as const }));
    renderWith({ initialIdentities: [identity()] }, fakeApi({ disconnect }));
    fireEvent.click(screen.getByRole("button", { name: "Disconnect Google" }));
    const group = screen.getByRole("group", { name: /confirm disconnecting google/i });
    expect(group).toHaveTextContent(/still sign in with an emailed code/i);
    expect(disconnect).not.toHaveBeenCalled();
    fireEvent.click(within(group).getByRole("button", { name: "Keep it" }));
    expect(screen.queryByRole("group", { name: /confirm/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Disconnect Google" })).toBeInTheDocument();
    expect(disconnect).not.toHaveBeenCalled();
  });

  it("disconnects after confirmation and shows the refreshed list", async () => {
    const disconnect = vi.fn(async () => ({ ok: true as const }));
    const listIdentities = vi.fn(async () => ({ ok: true as const, identities: [] }));
    renderWith(
      { providers: ["google"], initialIdentities: [identity()] },
      fakeApi({ disconnect, listIdentities }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Disconnect Google" }));
    fireEvent.click(screen.getByRole("button", { name: /yes, disconnect google/i }));
    expect(await screen.findByRole("button", { name: "Connect Google" })).toBeInTheDocument();
    expect(disconnect).toHaveBeenCalledWith(CSRF, "google");
    expect(listIdentities).toHaveBeenCalledOnce();
    expect(screen.getByText("Not connected")).toBeInTheDocument();
  });

  it("treats an already-removed account as done", async () => {
    renderWith(
      { initialIdentities: [identity()] },
      fakeApi({ disconnect: fail({ ok: false, kind: "not_found" }) }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Disconnect Google" }));
    fireEvent.click(screen.getByRole("button", { name: /yes, disconnect google/i }));
    await waitFor(() =>
      expect(screen.queryByRole("group", { name: /confirm/i })).not.toBeInTheDocument(),
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("shows a disconnect failure and keeps the account listed", async () => {
    renderWith(
      { initialIdentities: [identity()] },
      fakeApi({ disconnect: fail({ ok: false, kind: "unavailable" }) }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Disconnect Google" }));
    fireEvent.click(screen.getByRole("button", { name: /yes, disconnect google/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/something went wrong/i);
    expect(screen.getByRole("button", { name: "Disconnect Google" })).toBeInTheDocument();
  });

  it("shows why a connect attempt failed, as text", () => {
    renderWith({
      notice: "That provider account is already connected to another Readmely account.",
    });
    expect(screen.getByRole("alert")).toHaveTextContent(/already connected to another/i);
  });

  it("shows no notice when there is none", () => {
    renderWith({ providers: ["google"] });
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
