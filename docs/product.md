# Product

**Mentorix** is a backend for **trainers** and **clients**.

## Roles

`admin`, `trainer`, `client`. The `admin` role is exclusive (does not combine with others, enforced by a database trigger); `trainer` and `client` can combine (a trainer can be a client of another trainer).

## Unified account

One `user_id` per person; login via email and password (REST). Data stored in `users` and `auth_identities`. Domains and middleware rely on `user_id`.

## Connections

- Trainer accesses the system via web/REST with email and password.
- Client uses the Telegram bot (primary channel); invited via deep link from a trainer.
- A client can work with multiple trainers (`trainer_clients`).
- One client account serves all their trainers, not a separate account per trainer.

## Catalogs and programs

- **Exercises** — shared catalog; see [features/exercises.md](features/exercises.md).
- **Programs** — trainer templates; see [features/programs.md](features/programs.md).
