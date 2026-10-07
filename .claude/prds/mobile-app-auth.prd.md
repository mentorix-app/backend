# Mobile app sign-in and sign-up

## Problem

Clients use Mentorix only through the Telegram bot, and their accounts exist only as Telegram identities with no other way to sign in. The bot's menu and inline buttons limit what the client experience can be, and a client without Telegram cannot join at all. A Flutter developer is starting the mobile app now and cannot build anything past the first screen until the backend offers sign-in and sign-up that work for a phone.

## Evidence

- Assumption: the bot interface limits the product. Needs validation via the share of active Telegram clients who move to the app once it is available.
- Assumption: dependence on Telegram costs clients. Needs validation via trainer interviews about clients who never connected.
- Observed in the codebase: trainers sign in with email and password in the web cabinet, clients are created only when they accept an invite in the bot, and the only client-facing HTTP endpoint is the signed analytics page.

## Users

- **Primary**: clients of a trainer. Some already train through the bot and have a program, a trainer link and a workout history. Others are new and have never used Mentorix.
- **Primary**: trainers. Some already have an email and password account in the web cabinet, others are new and sign up from the app.
- **Not for**: admins. They keep using the web cabinet.

## Hypothesis

We believe **sign-in with Apple or Google that lands every person in their one existing account** will **let trainers and clients move from the bot to the app without losing trainer links or history** for **trainers and their clients**.
We'll know we're right when **active Telegram clients sign in to the app under their existing account and no duplicate accounts need manual cleanup**.

## Success metrics

| Metric | Target | How measured |
|---|---|---|
| Active Telegram clients who signed in to the app under the same account | TBD, needs a target from the owner | Count of accounts holding both a Telegram identity and an app identity, over active Telegram clients |
| Duplicate accounts that needed manual merging | 0 | Support requests |
| Flutter developer builds sign-in and sign-up on stage without backend changes | Yes | Developer confirmation against the published API contract |

## Account rules

1. A person has one account and any number of ways to sign in to it: Apple, Google, email with password, Telegram. A way to sign in belongs to one account only.
2. A new account is created in one case only: the person signs in with a way we have never seen and presents neither a code nor an open session.
3. Any two accounts of the same person can be joined with a link code. The channel where the person is already signed in (the bot or the app) issues the code, and the person enters it in the other. That proves they own both.

Two kinds of codes exist, both one-time and short-lived. An invite code comes from a trainer and links a client to that trainer. A link code comes from the bot or the app and joins two ways of signing in for one person.

## Decisions

- **Email matches an existing account.** When someone signs in with Apple or Google and the email matches an existing account, no account is created and nothing is joined automatically. The app asks them to sign in with the password once and then attaches Apple or Google. Web registration does not confirm email, so joining by email alone would let someone take over an account by registering another person's address first.
- **Trainer sign-up in the app.** Allowed. A first sign-in with no code asks "I am a trainer" or "I am a client". A new trainer gets the free plan. A new client waits for an invite code.
- **Joining two accounts that both hold data.** Done automatically. Trainer links, program assignments and workout history from both end up in one account and the other is removed. Nobody contacts support.
- **Two roles.** One account can be both a trainer and another trainer's client.

## Scope

**MVP**

- Sign-in with Apple or Google. A first sign-in creates an account, later sign-ins return the same one.
- Sign-in with email and password in the app for trainers who already use it on the web, and sign-up by the same means.
- Adding another way to sign in from inside the app while signed in.
- Role choice on a first sign-in without a code.
- Invite codes accepted in the app as well as in the bot. A new client no longer needs Telegram.
- Link codes issued by the bot ("Open in the app") and by the app, entered in the app before or after signing in. When the code arrives together with a first sign-in, no new account is created.
- "Connect Telegram" from the app, so a client who joined through the app is recognized by the bot.
- Automatic joining of two accounts, including when both hold data.
- A button opens the app with the code filled in when the app is installed. When it is not, the person installs the app and types the code by hand.
- The bot keeps working for every client who has not moved, and for clients who use both.

**Out of scope**

- Program view, workout marking and statistics for the app. That is a separate PRD; this one ends when the person is signed in and sits in the right account.
- Mobile push notifications. Notifications keep going through Telegram for now.
- Password reset and email confirmation. Both need a mail service that does not exist yet.
- Passing the code through an app store install automatically. It needs a third-party service and is unreliable on iOS, so the typed code covers that case.
- Sign-in by phone number and SMS. It needs a paid provider.
- Changing what the bot already does.

## Scenarios

In every row the person ends with exactly one account, and every trainer link and workout record they had stays on it.

### Clients

| # | Starting point | What the client does | Result |
|---|---|---|---|
| C1 | New, no app, has a trainer's invite | Follows the invite to the store, installs, signs in with Apple or Google, types the invite code | New account, linked to the trainer |
| C2 | New, app installed, has a trainer's invite | Taps the invite, the app opens with the code filled in, signs in | New account, linked to the trainer |
| C3 | New, installs the app with no invite | Signs in, chooses "I am a client" | New account with no trainer; the app offers "I have a code from my trainer" and "I already train through Telegram" |
| C4 | Has an app account, gets an invite from another trainer | Taps the invite or types the code | Same account, second trainer linked |
| C5 | Trains through the bot, no app | Takes a link code in the bot, installs, signs in, types the code | App sign-in attached to the existing account; no second account is created |
| C6 | Trains through the bot, app installed, not signed in | Taps the bot button, the app opens with the code, signs in | Same as C5 |
| C7 | Trains through the bot, signed in to the app first without a code | Enters the bot's link code afterwards | The two accounts are joined |
| C8 | Trains through the bot, accepted a new trainer's invite in the app before moving | Enters the bot's link code afterwards | One account holding the trainers and history from both |
| C9 | Joined through the app, later opens the bot | Taps "Connect Telegram" in the app and continues in the bot | Telegram attached to the same account; if the bot had already created a separate one, the two are joined |
| C10 | Has an app account without Telegram connected, accepts an invite in the bot | Accepts in the bot as today | The bot creates a Telegram account; a link code joins it with the app account |
| C11 | Second phone, same Apple or Google account | Signs in | Same account |
| C12 | Signed in with Apple on one phone and with Google on another | Takes a link code in the first app, enters it in the second | The two accounts are joined |
| C13 | Any, the code is wrong, expired or already used | Enters the code | Clear refusal, nothing changes, a new code can be requested |
| C14 | Signed in to an account that already has Telegram | Enters a link code issued to a different Telegram user | Refused; two Telegram users are never joined |
| C15 | Moved, still opens the bot | Uses either channel | Both show the same program, trainers and history |

### Trainers

| # | Starting point | What the trainer does | Result |
|---|---|---|---|
| T1 | Has a web account | Signs in to the app with the same email and password | Same account |
| T2 | Has a web account, signed in to the app | Adds Apple or Google in the profile | Same account, more ways to sign in |
| T3 | Has a web account, taps Apple or Google on the sign-in screen first, same email | Sees "An account with this email exists, sign in with your password once", does so | Apple or Google attached to the existing account; no second account |
| T4 | New trainer | Signs in with Apple or Google, or signs up with email and password, and chooses "I am a trainer" | New trainer account on the free plan |
| T5 | Trainer who trains with another trainer | Enters that trainer's invite code in their own account | Same account, now also a client |
| T6 | Trainer signed in with Apple, whose Apple account hides the real email, also has a web account | Signs in, gets a new account, then takes a link code in one and enters it in the other | The two accounts are joined |

## Delivery milestones
<!-- Business outcomes, not engineering tasks. /plan turns each into a plan. -->
<!-- Status: pending | in-progress | complete -->

| # | Milestone | Outcome | Status | Plan |
|---|---|---|---|---|
| 1 | Sign-in on the phone | A trainer signs in to the app with their web email and password and stays signed in between launches; a new person signs up the same way | complete | `.claude/plans/mobile-app-auth.plan.md` |
| 2 | Apple and Google | Anyone creates an account or returns to it with Apple or Google, chooses a role on first sign-in, and can add another way to sign in from the profile | pending | n/a |
| 3 | Invites in the app | A new client with no Telegram enters a trainer's code in the app and appears in that trainer's client list | pending | n/a |
| 4 | Moving from Telegram | A client takes a link code from the bot, enters it in the app and sees the same trainers and history; an account created by mistake is joined automatically | pending | n/a |
| 5 | Telegram from the app | A client who joined through the app connects Telegram and the bot recognizes them | pending | n/a |

## Open questions

- [ ] App Store review requires account deletion from inside the app for apps that create accounts. Does deletion belong to this PRD or the next one?
- [ ] Apple requires Sign in with Apple when an iOS app offers another third-party sign-in. Confirm Android ships with Google only or with both.
- [ ] How long does a code live, and how many can a person request?
- [ ] Who tells existing clients to move: a bot broadcast, the trainer, or both?
- [ ] When two accounts are joined and both are trainers with their own programs and clients, do we join them or refuse? The decisions above cover client data only.
- [ ] Can a client have email and password, or only Apple and Google? A client who loses their Apple or Google account has no recovery path except Telegram.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| A person signs in without a code and gets a second account | High | Medium | The app's first screen offers both code paths, and a link code joins the accounts at any time |
| A leaked or guessed code attaches a stranger's sign-in to an account | Low | High | Codes are one-time and short-lived, and attempts are rate limited |
| Joining two accounts loses or duplicates trainer links, assignments or history | Medium | High | The join happens as one atomic operation and is covered by tests for every scenario above |
| A client counts twice against a trainer's plan quota while they have two accounts | Medium | Medium | Verify quota counting during planning; joining removes the second count |
| App Store rejects the build over missing account deletion or missing Sign in with Apple | Medium | High | Resolve both open questions before the first store submission |

---
*Status: DRAFT. Requirements only; implementation planning pending via /plan.*
