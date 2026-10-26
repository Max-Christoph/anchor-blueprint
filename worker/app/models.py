"""Pydantic-Schemas der Worker-API."""

from __future__ import annotations

import math
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator

from app.settings import settings


class CalcRequest(BaseModel):
    """Eingabe fuer POST /process/calc."""

    model_config = ConfigDict(extra="forbid", str_strip_whitespace=True)

    values: list[float] = Field(min_length=1, description="Wertereihe (endliche Zahlen)")
    operation: Literal["summary"] = Field(default="summary", description="Berechnungsart")

    @field_validator("values")
    @classmethod
    def validate_values(cls, values: list[float]) -> list[float]:
        """Begrenzt Laenge und schliesst NaN/Infinity aus."""

        if len(values) > settings.max_values:
            raise ValueError(f"maximal {settings.max_values} Werte erlaubt, erhalten: {len(values)}")
        for value in values:
            if math.isnan(value) or math.isinf(value):
                raise ValueError("nur endliche Zahlen erlaubt")
        return values


class CalcResponse(BaseModel):
    """Ergebnis von POST /process/calc."""

    operation: Literal["summary"]
    count: int
    sum: float
    mean: float
    median: float
    min: float
    max: float
    stddev: float
    p90: float
    duration_ms: float


class HealthResponse(BaseModel):
    """Antwort von GET /healthz."""

    status: Literal["ok"]
    service: str
    version: str
