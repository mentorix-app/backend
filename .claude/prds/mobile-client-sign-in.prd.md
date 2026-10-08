# Mobile client sign-in

## Problem

Clients reach their trainer only through the Telegram bot, and the backend has no way to sign a client in from a mobile app. The Flutter app cannot start without it. Clients who already train through Telegram must end up in the same account when they move to the app, otherwise their history and trainer links are split across two accounts.

## Evidence

- Stage, 2026-10-08: 26 trainer-client links, every one created by a Telegram invite. There is no other way to become a client today.
- The API offers email and password sign-in only, and it is used by trainers in the web cabinet. Clients have no credentials at all.
- The first attempt (2026-10-07) covered trainers, email, Apple, Google, account merging and session hardening at once. The owner reverted it on 2026-10-08 as too complex.
- Demand from clients for an app: assumption, needs validation with the trainers who use stage now.

## Users

- **Primary**: a client of a trainer. Either new (never used the bot) or existing (trains through the Telegram bot today).
- **Not for**: trainers and admins. They stay in the web cabinet with email and password. A trainer who signs in to the app gets a separate client account; nothing joins accounts by email.

## Hypothesis

We believe that Google and Apple sign-in, plus a Telegram link in settings, will let clients use the app with one account each. We'll know we're right when an existing Telegram client signs in to the app, links Telegram, and sees the same trainer and history as in the bot, with no second account left behind.

## Success Metrics

| Metric | Target | How measured |
|---|---|---|
| Existing Telegram clients who link end up with one account | 100% of successful links | count of users per Telegram id and per Google/Apple id in the database |
| Web cabinet and bot behave as before | no change | existing test suites, cabinet check on stage after each deploy |
| Refused links (both accounts hold data) | rare; number TBD after the first month | log count of the refusal |
| Clients signed in through the app | TBD, depends on the app release date | count of Google and Apple identities |

## Scope

**MVP**: the smallest set that lets a client own one account across the app and Telegram.

- Sign in and sign up in the app with Google or Apple. The first sign-in creates a client account.
- The app session survives an app restart and can be ended by the client (sign out). The web cabinet session works exactly as today.
- A client joins a trainer by entering or opening the trainer's existing invite in the app. The plan's client limit applies as it does in the bot.
- A client links Telegram from the app settings by opening the bot and confirming there.
  - Telegram id not known yet: it is attached to the current account, and the bot starts working for that account.
  - Telegram id already has an account and the app account is empty: the client continues in the old account, now with Google or Apple sign-in; the empty one is removed.
  - Both accounts hold data (a trainer link or a recorded workout): the link is refused with a clear message and resolved by hand.
- The profile tells the app whether Telegram is linked, so the app can ask "already training through Telegram?" right after the first sign-in.

**Out of scope**

- Client screens API (program, workout of the day, marking completion, comments): a separate plan after this one.
- Trainers and admins in the app.
- Email and password sign-in in the app; attaching a second sign-in method other than Telegram.
- Automatic merge of two accounts that both hold data.
- Joining accounts by matching email.
- Session hardening beyond what exists today (token reuse detection, session families, absolute lifetime): revisit after the app is live.
- Unlinking Telegram.
- Deleting the account from inside the app: a separate plan, needed before App Store review.

## Delivery Milestones
<!-- Business outcomes, not engineering tasks. /plan turns each into a plan. -->
<!-- Status: pending | in-progress | complete -->

| # | Milestone | Outcome | Status | Plan |
|---|---|---|---|---|
| 1 | Google sign-in | A person signs in to the app with Google, gets a client account, stays signed in after a restart, can sign out | complete | `.claude/plans/mobile-client-sign-in-google.plan.md` |
| 2 | Apple sign-in | The same with Apple | in-progress | `.claude/plans/mobile-client-sign-in-apple.plan.md` |
| 3 | Telegram link | A client links Telegram from settings; an existing Telegram client lands in the old account | pending | |
| 4 | Invite in the app | A client without Telegram joins a trainer with the trainer's invite | pending | |

## Open Questions

- [ ] Android return page for Apple sign-in: Apple posts the result to a web address that must send the browser back to the app. Needs the Flutter package's expected link format and the Android package name from the Flutter developer; a small follow-up PR.
- [ ] Google and Apple client ids do not exist. Who creates them and when? Milestones 1 and 2 can be built and tested without them but cannot be checked against real sign-in.
- [x] Platforms at launch: iOS and Android together (owner, 2026-10-08). Apple sign-in on Android goes through a web form.
- [x] In-app account deletion, required by App Store review: a separate plan before the store submission (owner, 2026-10-08).
- [ ] Text of the refusal message and where it sends the client (trainer, support contact).
- [ ] How long an app session lives without use before the client must sign in again.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Scope grows again through review rounds | Medium | High | Every addition outside this document needs the owner's decision; one milestone per small pull request |
| A link moves sign-in to the wrong account | Low | High | The link is confirmed inside the client's own Telegram; a merge happens only when the app account is empty |
| A change breaks sign-in for trainers on stage | Low | High | Web session behaviour is not changed; cabinet check after each deploy |
| Stage data loss during a schema change | Low | High | Additive changes only; restore-tested backup and the owner's go-ahead before every write |
| Client ids arrive late | Medium | Medium | Build against test keys; real sign-in check becomes the last step of milestones 1 and 2 |

---
*Status: DRAFT. Requirements only; implementation planning pending via /plan.*
