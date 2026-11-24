# Phase 9 · UI improvement

**Status:** Done · PR #28 · **Last updated:** 2026-11-25 · Branch `feat/ui-redesign` · Parent plan: [v1-roadmap.md](v1-roadmap.md)

Phases 2 to 8 built every feature with plain black-and-white cards. This phase gives the whole product one look: a design system taken from the approved landing-page design, a small set of shared components, and every screen rebuilt on them. No API, database or worker change is part of it.

## 1. Order of work

1. **Screens:** the list below is settled first, so nothing is designed twice.
2. **Design system:** colour, type, shape, motion and themes as tokens.
3. **Components:** every shared part built once, with all its states.
4. **Screens, one at a time:** public pages first, then the signed-in app. Each is driven in a browser in light and dark, at desktop and phone width, before the next starts.

## 2. Screens

| # | Screen | Route | State today | Change |
|---|---|---|---|---|
| 1 | Landing | `/` | Shows system status | New. Shows the landing page for everyone; a signed-in visitor sees "Open dashboard" |
| 2 | System status | `/status` | At `/` | Moved, restyled |
| 3 | Sign in | `/login` | Built | Restyled; an emailed code lands on `/dashboard`; provider sign-in still lands on `/account` (the API sets it) |
| 4 | Privacy policy | `/privacy` | None | New |
| 5 | Terms of service | `/terms` | None | New |
| 6 | Account deleted | `/goodbye` | Built | Restyled |
| 7 | Not found, error, loading | `not-found`, `error`, `loading` | Next.js defaults | New |
| 8 | Dashboard | `/dashboard` | Built | Restyled, project count, output icons, low-credit notice, loading skeleton |
| 9 | New project | `/projects/new` | Built | Restyled, cost note |
| 10 | Project overview | `/projects/[id]` | One long page | Header with tabs; overview holds the summary and analysis facts |
| 11 | README | `/projects/[id]/readme` | A card on the project page | Own route; editor and preview side by side |
| 12 | Context pack | `/projects/[id]/context-pack` | A card | Own route |
| 13 | Diagram | `/projects/[id]/diagram` | A card | Own route; zoom, fit and pan |
| 14 | Project settings | `/projects/[id]/settings` | Delete card at the bottom | Own route: analyse again, details, delete |
| 15 | Usage | `/usage` | Built | Restyled, cost table |
| 16 | Account | `/account` | Built | Restyled |

Not built: payments, plans, anything listed as out of scope for version 1 in the roadmap.

## 3. Design system

Taken from the landing-page design, with the same names where one exists.

| Area | Decision |
|---|---|
| Accent | One restrained teal: `#087c72` on light, `#58c3b6` on dark. Soft tint behind icons and badges |
| Neutrals | Cool greys: page `#fbfcfd` / `#101719`, surface `#ffffff` / `#151f22`, raised surface and lines in the same family. A deep navy (`#132b30`) band for the sample and the final call to action |
| Status | `ok` green, `warn` amber, `bad` red; a status is always a word or icon as well as a colour |
| Type | Manrope for the interface, DM Mono for code, paths, the Markdown editor and the six-digit code. Loaded through `next/font`, so no request leaves the site (the content security policy allows only `self`). Page title 24 px, card title 18 px, body 14 px, caption 12 px; headings use tight letter spacing |
| Shape | Cards 12 px radius, 1 px border, soft shadow. Buttons and inputs 40 px high, 7 px radius |
| Themes | Light and dark, following the system until the visitor picks one; the choice is remembered. Set before first paint so there is no flash; the script carries the request's CSP nonce |
| Diagram kinds | Client blue-violet, gateway purple, service blue, worker cyan, data store green, cache yellow, queue orange, external grey |
| Motion | Short and quiet: entrance on first paint through CSS keyframes, scroll reveal, tab indicator, accordion, progress and menus through Motion. Everything respects `prefers-reduced-motion` |
| Accessibility | 4.5:1 text contrast, visible focus rings, a visible label on every field, errors announced, no meaning by colour alone |

### Content security policy

The policy has no `unsafe-inline` for styles. Rules for the new UI: styles come from the stylesheet, not `style` attributes in server markup; animation that needs a computed value (a bar width, a transform) is set by Motion in the browser, which is allowed; nothing loads a font, script or image from another origin.

## 4. Libraries

| Library | For | Why this one |
|---|---|---|
| `radix-ui` | Tabs, accordion, dropdown menu, radio group, tooltip, switch | Accessible behaviour (keyboard, focus, ARIA) without a look of its own; the shadcn components already in `components.json` are built on it |
| `motion` | Entrance, layout and presence animation | The standard React animation library; respects reduced motion with `MotionConfig` |
| `lucide-react` | Icons | Already in use |
| `next/font` | Manrope and DM Mono | Self-hosted at build time |

Brand marks (GitHub, Google) are small inline SVGs, because the icon set no longer ships brand logos. No toast library: messages stay inline, as the brief says. No new tests are written (fast-forward rule); the existing suite stays green.

## 5. Components

Built once in `src/components/ui/`, in this order, each with its states.

| Group | Components |
|---|---|
| Actions | Button (primary, secondary, danger, ghost, link; sizes; busy), icon button, copy button with "Copied" for two seconds |
| Forms | Input, textarea, labelled field with hint and error, radio card group, code input (six digits, monospace) |
| Display | Card (with title, helper, footer), badge and status badge, progress bar with step label, skeleton, empty state, inline message (info, success, error), failure note, key-value list, code and path chip, language bar, stat |
| Navigation | Tabs (route-aware, animated indicator), dropdown menu, accordion, breadcrumb |
| Feedback | Inline confirm panel, tooltip |
| Motion helpers | `Reveal` (scroll entrance), `Stagger`, animated number |
| Layout | Container, page header, section header, app shell (top bar with credit pill, theme toggle, user menu; phone menu), site header and footer, logo mark |
| Content | Markdown view for README and context-pack preview, code block |

## 6. Project workspace

`/projects/[id]` becomes a layout: a header (name, status, "View on GitHub") and a tab bar, with one route per tab. The project and its polling live in one provider so every tab sees the same live state, and the old single-page component is retired. The panels keep their data logic; their markup moves onto the shared components.

## 7. Verification

No new tests. For every screen: drive it in a browser against the fake API in `e2e/`, in both themes, at 1280 px and 360 px wide; check the console for content-security-policy violations; run `pnpm lint`, `pnpm test`, `pnpm build` and the e2e suite. The pull request lists what was checked.

## 8. Risks

| Risk | Plan |
|---|---|
| Inline `style` blocked by the CSP | Rules in section 3; verified in the browser console |
| Existing unit tests look for old wording | Keep user-facing wording unless the design needs it; update a test only when its subject was deliberately changed |
| Scope: sixteen screens | Fixed order; one commit per screen group; a screen is not started before the previous one is checked |
| The landing page's sample data must be real | Figures (limits, costs, credits) come from the product, not the mock-up; see section 9 |

## 9. Data on the landing page

The mock-up had a few claims that must match the product. These are the corrections applied while porting it.

| Mock-up | Product |
|---|---|
| "No credit card" | Kept: there is no payment step |
| Hero "Get started" only | Hero takes a repository address and goes to sign-in with it kept, as the brief says |
| "10-point health check", "Full version history" | Correct (ten structure checks; every edit is a version) |
| Cost list (10, 15, 15, free) | Correct; analysis usually settles near 1 credit |
| Diagram "editable-quality SVG" | Reworded: SVG and PNG download; hand editing is not in version 1 |
| "Built for GitHub, Claude Code, Cursor, VS Code" | Reduced to tools the context pack really targets: Claude Code, Cursor, AGENTS.md |
| Sample repository "acme / streambase" | Marked as a sample; its figures are illustrative and labelled so |
| Footer "All systems operational" | Reads the real status instead of a fixed phrase |
| Privacy wording in the trust strip | Uses the approved wording: the app stores the readable text of analysed files until the project or account is deleted |
