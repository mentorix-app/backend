# Product

**Mentorix** is a backend for **trainers** and **clients**.

## Roles

`admin`, `trainer`, `client`. The `admin` role is exclusive (does not combine with others, enforced by a database trigger); `trainer` and `client` can combine (a trainer can be a client of another trainer).

## Unified account

One `user_id` per person; sign-in by email and password, by Apple or Google ID token (REST), or through the Telegram bot. Data stored in `users` and `auth_identities`. Domains and middleware rely on `user_id`.

## Connections

- Trainer accesses the system via web/REST with email and password, or signs in with Apple or Google from the mobile app and picks the trainer role.
- Client uses the Telegram bot (primary channel; invited via deep link from a trainer), or signs in with Apple or Google from the mobile app and picks the client role.
- A client can work with multiple trainers (`trainer_clients`).
- One client account serves all their trainers, not a separate account per trainer.

## Catalogs and programs

- **Exercises** — shared catalog; see [features/exercises.md](features/exercises.md).
- **Programs** — trainer templates; see [features/programs.md](features/programs.md).
