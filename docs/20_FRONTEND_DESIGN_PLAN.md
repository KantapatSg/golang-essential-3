# Frontend design plan (two-pass)

## Pass 1 — compact direction

Subject: a task operations portfolio for a technical interviewer. The page's job is to make reliability boundaries tangible in under one minute.

- Palette: signal slate `#101619`, panel moss `#172126`, acid protocol `#C3FF61`, coral warning `#FF795D`, trace blue `#87D5FF`.
- Type: Manrope for readable interface/body, DM Mono for operational labels and data, with oversized Manrope display headlines to make the portfolio feel editorial rather than like an admin template.
- Layout: quiet shell + dense operational rows. The landing hero pairs a short thesis with a tilted “LIVE ARCHITECTURE” signal card; workspace pages use a left rail/form and a readable event/list lane.
- Signature: the acid-green signal line and coral “event boundary” accent; they encode healthy flow vs. attention, rather than decorating a generic dashboard.

Wireframe:

```text
[ S signal ledger ]                         [ Enter workspace ]

  TRACE THE WORK.            [ LIVE ARCHITECTURE ]
  TRUST THE SIGNAL.          Browser -> Gateway -> Services
  short thesis               ─────────●──────────────
  [ open workspace ]

  [01 transactional] [02 events] [03 signals]
```

## Pass 2 — critique and decision

The first instinct was a cream/serif portfolio, but that default would flatten the operational subject into a marketing page. It was revised to a dark signal-slate field with mono labels, an acid health line, and a single coral boundary. Motion is limited to button lift and is disabled for reduced-motion users. Empty/error/loading states explain what to do next and never expose raw tokens or stack traces.

Implemented routes: `/`, `/login`, `/app`, `/app/tasks`, and admin-only `/app/activities`, `/app/analytics`, `/app/system`. Analytics is intentionally honest about its current projection state until ClickHouse is connected.
