# Identity Workspace UX Contract

This document records durable interaction ownership for the web product. Visual intent and tokens live in [DESIGN.md](DESIGN.md); authorization and persisted-state rules remain owned by the backend.

## Product register

- Audience: Russian-speaking individuals and small project groups.
- Primary devices: mobile PWA for frequent short sessions; desktop for project and administration work.
- Locale: product-owned UI is `ru-RU`; dates and numbers use the active product locale.
- Accessibility target: WCAG 2.2 AA for new and touched workflows.
- State authority: PostgreSQL is authoritative. Browser state is limited to presentation, navigation, drafts and display-only timer ticks.

## Canonical UI Map

| Capability | Canonical owner | Source of truth | Allowed variants | Verification |
| --- | --- | --- | --- | --- |
| Select/Listbox | Native `select` for OS-owned menus; feature-owned authored picker when richer semantics are required | `frontend/src/presentation` and `frontend/src/App.tsx` | native, authored business picker | keyboard, touch and narrow viewport |
| Date | Native date/time inputs plus the task calendar for application-owned browsing | `frontend/src/presentation/tasks` and task calendar in `frontend/src/App.tsx` | native date/time, authored task calendar | locale, keyboard and mobile viewport |
| Form | Native form with `noValidate` and feature submit handler | feature presentation module plus application/backend validation | create, edit, settings | frontend build and feature tests |
| Scrollbar | Global application stylesheet | `frontend/src/styles.css` | thin default, hidden geometry-only rail | computed style and narrow viewport |
| CRUD | Typed HTTP adapter and owning feature module | `frontend/src/infrastructure/http/apiClient.ts` plus backend use case | return-to-list, stay-in-context | Go tests and frontend production build |

## Shared behavior

- Use native `button`, `a`, `form`, `input`, `select` and `textarea` semantics before authored equivalents.
- Product forms own validation copy and therefore declare `noValidate`; backend validation remains authoritative.
- Successful mutations refresh or patch the owning view without inventing browser-local persisted state.
- Busy controls keep their geometry, prevent duplicate submission and expose a visible in-progress label.
- Errors stay close to the failed workflow and preserve the user's entered values when recovery is possible.
- Mobile sheets/dialogs keep actions reachable above safe areas and the virtual keyboard.
- Reduced-motion preferences disable nonessential transitions; themes may change tokens, not behavior.

## Desktop shell boundary

The Electron controls under `desktop/controls/` are platform chrome, not product-page UI. Their local preload owns the three window actions; the remote workspace receives no IPC bridge or Node.js access. The desktop policy tests are the canonical verification for that boundary.
