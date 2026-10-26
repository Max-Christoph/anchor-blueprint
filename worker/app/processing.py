"""Reine Berechnungsfunktionen ohne Seiteneffekte."""

from __future__ import annotations

import math
from collections.abc import Sequence
from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class Summary:
    """Aggregierte Kennzahlen einer Wertereihe."""

    count: int
    total: float
    mean: float
    median: float
    minimum: float
    maximum: float
    stddev: float
    p90: float


def percentile(sorted_values: Sequence[float], quantile: float) -> float:
    """Lineare Interpolation (R-7) auf bereits sortierten Werten."""

    if not sorted_values:
        raise ValueError("sorted_values darf nicht leer sein")
    if not 0.0 <= quantile <= 1.0:
        raise ValueError(f"quantile muss in [0, 1] liegen, war: {quantile}")

    if len(sorted_values) == 1:
        return float(sorted_values[0])

    position: float = quantile * (len(sorted_values) - 1)
    lower_index: int = math.floor(position)
    upper_index: int = math.ceil(position)
    lower_value: float = float(sorted_values[lower_index])
    upper_value: float = float(sorted_values[upper_index])
    fraction: float = position - lower_index
    return lower_value + (upper_value - lower_value) * fraction


def summarize(values: Sequence[float]) -> Summary:
    """Berechnet Kennzahlen fuer eine nicht-leere Wertereihe."""

    if not values:
        raise ValueError("values darf nicht leer sein")
    for value in values:
        if math.isnan(value) or math.isinf(value):
            raise ValueError("values darf nur endliche Zahlen enthalten")

    ordered: list[float] = sorted(float(value) for value in values)
    count: int = len(ordered)
    total: float = math.fsum(ordered)
    mean: float = total / count

    variance: float = math.fsum((value - mean) ** 2 for value in ordered) / count
    stddev: float = math.sqrt(variance)

    if count % 2 == 1:
        median: float = ordered[count // 2]
    else:
        median = (ordered[count // 2 - 1] + ordered[count // 2]) / 2.0

    return Summary(
        count=count,
        total=round(total, 6),
        mean=round(mean, 6),
        median=round(median, 6),
        minimum=round(ordered[0], 6),
        maximum=round(ordered[-1], 6),
        stddev=round(stddev, 6),
        p90=round(percentile(ordered, 0.90), 6),
    )
