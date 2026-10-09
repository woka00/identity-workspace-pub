# Frontend architecture

The frontend follows the same inward dependency rule as the backend:

```text
presentation (React UI)
       │
       ├────► application (use cases and pure workflows)
       ├────► domain (API-independent models)
       └────► infrastructure (HTTP, browser and export adapters)

application ─────► domain
infrastructure ──► domain
domain ──────────► no project layer
```

## Directories

- `src/domain` contains data models shared by the UI and adapters.
- `src/application` contains deterministic product rules such as formal date
  boundaries and nutrition-goal calculations.
- `src/infrastructure` contains communication with the backend and browser or
  third-party adapters.
- React composition remains in `App.tsx`; UI modules can be moved into
  `src/presentation` as they become independently reusable.
- `src/presentation/time-tracker` owns the time-tracker screen and its transient
  animation/timer state. Persisted activities and session totals always come from
  the HTTP adapter; the one-second browser tick is display-only and is reconciled
  with the backend on focus, visibility changes, and while a session is active.
  The statistics sub-screen consumes server-aggregated day/week/month projections
  and keeps only navigation, period selection and presentation state in React.
- `src/presentation/nutrition` owns the camera scanner and catalogue moderation
  screens. Candidate and review data are loaded through the HTTP adapter; access
  control and catalogue visibility remain server responsibilities.

All backend URLs stay relative to preserve same-origin cookies in both Safari
and the installed PWA. The service worker and manifest remain public entry
points and must not depend on application bundles.

`npm run check:architecture` enforces the inward dependency direction.
