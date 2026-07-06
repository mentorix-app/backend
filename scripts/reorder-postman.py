#!/usr/bin/env python3
"""Reorder Postman collection for linear walkthrough execution."""
from __future__ import annotations

import copy
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
COLLECTION = ROOT / "postman/mentorix-backend.postman_collection.json"


def find_folder(items: list, name: str) -> dict:
    for it in items:
        if it["name"] == name:
            return it
    raise KeyError(name)


def find_request(folder: dict, name: str) -> dict:
    for it in folder["item"]:
        if it.get("request") and it["name"] == name:
            return it
    raise KeyError(f"{folder['name']}/{name}")


def pick(folder: dict, names: list[str]) -> list[dict]:
    by_name = {it["name"]: it for it in folder["item"] if "request" in it}
    missing = [n for n in names if n not in by_name]
    if missing:
        raise KeyError(f"missing in {folder['name']}: {missing}")
    return [by_name[n] for n in names]


def make_examples_folder(folder_name: str, requests: list[dict]) -> dict:
    return {"name": folder_name, "item": requests}


def clone_block_request(req: dict) -> dict:
    out = copy.deepcopy(req)
    out["name"] = "POST /programs/:id/weeks/:week_id/days/:day_id/blocks (2nd block)"
    out["request"]["description"] = (
        "Второй single-блок в том же дне (для merge). "
        "Сохраняет program_week_day_block_id_2."
    )
    test = {
        "listen": "test",
        "script": {
            "type": "text/javascript",
            "exec": [
                "pm.test('status 201', () => pm.response.to.have.status(201));",
                "if (pm.response.code === 201) {",
                "    const b = pm.response.json();",
                "    const week = (b.weeks || []).find(w => w.id === pm.environment.get('program_week_id'));",
                "    const day = week && week.days ? week.days.find(d => d.id === pm.environment.get('program_week_day_id')) : null;",
                "    if (day && day.blocks && day.blocks.length > 0) {",
                "        const block = day.blocks[day.blocks.length - 1];",
                "        pm.environment.set('program_week_day_block_id_2', block.id);",
                "    }",
                "}",
            ],
        },
    }
    out["event"] = [e for e in out.get("event", []) if e["listen"] != "test"] + [test]
    return out


def main() -> None:
    data = json.loads(COLLECTION.read_text(encoding="utf-8"))

    data["info"]["description"] = (
        "Коллекция Mentorix Backend. **Выполняй папки сверху вниз**, внутри папки — запросы по порядку.\n\n"
        "## Environments\n"
        "- **Mentorix Local** — `http://localhost:8080`\n"
        "- **Mentorix Render Dev** — `https://mentorix-backend.onrender.com`\n\n"
        "## Линейный прогон\n"
        "1. **Health** → **Auth** (Login … Refresh)\n"
        "2. **Exercises** (GET подхватит exercise_id из сида)\n"
        "3. **Programs** — полный сценарий программы (блоки → reorder → publish → cleanup)\n"
        "4. **Admin** / **Trainer** — по необходимости (`client_user_id` задай вручную)\n"
        "5. **Teardown** — logout в самом конце\n\n"
        "Переменные заполняются test/prerequest скриптами; reorder body — через collection prerequest."
    )

    # --- Auth ---
    auth = find_folder(data["item"], "Auth")
    auth_order = [
        "POST /auth/login",
        "GET /auth/me",
        "PATCH /auth/me",
        "POST /auth/register",
        "POST /auth/refresh",
    ]
    teardown_reqs = pick(auth, ["POST /auth/logout", "POST /auth/logout-all"])
    auth["item"] = pick(auth, auth_order)

    # --- Exercises ---
    ex = find_folder(data["item"], "Exercises")
    ex_main = pick(
        ex,
        [
            "GET /exercises",
            "POST /exercises",
            "GET /exercises/:id",
            "PUT /exercises/:id",
            "DELETE /exercises",
        ],
    )
    ex_example = find_request(ex, "GET /exercises (with filters example)")
    ex["item"] = ex_main + [make_examples_folder("Examples", [ex_example])]

    # --- Programs ---
    prog = find_folder(data["item"], "Programs")
    blocks1 = find_request(prog, "POST /programs/:id/weeks/:week_id/days/:day_id/blocks")
    blocks2 = clone_block_request(blocks1)

    prog_main_names = [
        "GET /programs",
        "POST /programs",
        "GET /programs/:id",
        "PATCH /programs/:id",
        "POST /programs/:id/weeks/:week_id/days/:day_id/blocks",
        "POST /programs/:id/weeks/:week_id/days/:day_id/blocks (2nd block)",
        "PUT /programs/:id/weeks/:week_id/blocks/:block_id/exercises/:item_id",
        "PUT /programs/:id/weeks/:week_id/days/:day_id/blocks/reorder",
        "PUT /programs/:id/weeks/:week_id/blocks/:block_id/exercises/reorder",
        "PATCH /programs/:id/weeks/:week_id/blocks/:block_id",
        "POST /programs/:id/weeks/:week_id/days/:day_id/blocks/merge",
        "POST /programs/:id/weeks/:week_id/blocks/:block_id/exercises",
        "POST /programs/:id/weeks",
        "PUT /programs/:id/weeks/reorder",
        "PUT /programs/:id/weeks/:week_id/days/reorder",
        "POST /programs/:id/weeks/:week_id/days",
        "POST /programs/:id/weeks/:week_id/blocks/:block_id/move",
        "POST /programs/:id/weeks/:week_id/blocks/:block_id/exercises/:item_id/extract",
        "POST /programs/:id/weeks/:week_id/blocks/:block_id/exercises/:item_id/move",
        "POST /programs/:id/weeks/:week_id/blocks/:block_id/ungroup",
        "POST /programs/:id/publish",
        "POST /programs/:id/publish-update",
        "GET /programs/:id/assignments",
        "POST /programs/:id/assignments/sync",
        "GET /programs/:id/versions",
        "POST /programs/:id/versions/cleanup",
        "DELETE /programs/:id/versions/:version_id",
        "POST /programs/:id/archive",
        "DELETE /programs/:id/weeks/:week_id",
        "DELETE /programs/:id",
    ]
    by_name = {it["name"]: it for it in prog["item"] if "request" in it}
    by_name[blocks2["name"]] = blocks2
    prog_optional = pick(
        prog,
        [
            "DELETE /programs/:id/weeks/:week_id/blocks/:block_id/exercises/:item_id",
            "DELETE /programs/:id/weeks/:week_id/days/:day_id",
            "DELETE /programs/:id/weeks/:week_id/blocks/:block_id",
        ],
    )
    prog_example = find_request(prog, "GET /programs (with filters example)")
    prog["item"] = [by_name[n] for n in prog_main_names] + [
        make_examples_folder("Examples", [prog_example]),
        make_examples_folder("Optional / destructive", prog_optional),
    ]

    # --- Top-level order ---
    health = find_folder(data["item"], "Health")
    admin_folder = find_folder(data["item"], "Admin")
    trainer = find_folder(data["item"], "Trainer")
    data["item"] = [
        health,
        auth,
        ex,
        prog,
        admin_folder,
        trainer,
        {"name": "Teardown", "item": teardown_reqs},
    ]

    COLLECTION.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print("Reordered", COLLECTION)


if __name__ == "__main__":
    main()
