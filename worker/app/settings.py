"""Laufzeitkonfiguration des Workers (nur Umgebungsvariablen)."""

from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class Settings:
    """Unveraenderliche Worker-Einstellungen."""

    app_env: str
    log_level: str
    max_values: int
    service_name: str = "enterprise-tool-worker"


def _get_int(key: str, fallback: int) -> int:
    raw: str = os.environ.get(key, "").strip()
    if raw == "":
        return fallback
    try:
        return int(raw)
    except ValueError as exc:
        raise ValueError(f"{key} muss eine Ganzzahl sein, war: {raw!r}") from exc


def load_settings() -> Settings:
    """Liest die Settings aus der Umgebung und validiert sie."""

    max_values: int = _get_int("MAX_VALUES", 10000)
    if max_values < 1:
        raise ValueError(f"MAX_VALUES muss >= 1 sein, war: {max_values}")

    return Settings(
        app_env=os.environ.get("APP_ENV", "development").strip() or "development",
        log_level=os.environ.get("LOG_LEVEL", "info").strip().lower() or "info",
        max_values=max_values,
    )


settings: Settings = load_settings()
