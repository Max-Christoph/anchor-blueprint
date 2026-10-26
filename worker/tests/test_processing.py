"""Tests der reinen Berechnungsfunktionen."""

from __future__ import annotations

import math

import pytest

from app.processing import percentile, summarize

SAMPLE = [120.0, 340.0, 95.0, 480.0, 210.0, 365.0, 155.0, 290.0]


def test_summarize_sample() -> None:
    summary = summarize(SAMPLE)

    assert summary.count == 8
    assert summary.total == pytest.approx(2055.0)
    assert summary.mean == pytest.approx(256.875)
    assert summary.median == pytest.approx(250.0)
    assert summary.minimum == pytest.approx(95.0)
    assert summary.maximum == pytest.approx(480.0)
    assert summary.stddev == pytest.approx(125.994482, rel=1e-6)
    assert summary.p90 == pytest.approx(399.5)


def test_summarize_single_value() -> None:
    summary = summarize([42.0])

    assert summary.count == 1
    assert summary.total == pytest.approx(42.0)
    assert summary.mean == pytest.approx(42.0)
    assert summary.median == pytest.approx(42.0)
    assert summary.stddev == pytest.approx(0.0)
    assert summary.p90 == pytest.approx(42.0)


def test_summarize_even_and_odd_median() -> None:
    assert summarize([1.0, 2.0, 3.0, 4.0]).median == pytest.approx(2.5)
    assert summarize([1.0, 2.0, 10.0]).median == pytest.approx(2.0)


def test_summarize_rejects_empty() -> None:
    with pytest.raises(ValueError):
        summarize([])


def test_summarize_rejects_non_finite() -> None:
    with pytest.raises(ValueError):
        summarize([1.0, math.nan])
    with pytest.raises(ValueError):
        summarize([1.0, math.inf])


def test_summarize_is_order_independent() -> None:
    assert summarize([3.0, 1.0, 2.0]) == summarize([1.0, 2.0, 3.0])


def test_percentile_interpolates() -> None:
    ordered = [10.0, 20.0, 30.0, 40.0]

    assert percentile(ordered, 0.0) == pytest.approx(10.0)
    assert percentile(ordered, 0.5) == pytest.approx(25.0)
    assert percentile(ordered, 0.9) == pytest.approx(37.0)
    assert percentile(ordered, 1.0) == pytest.approx(40.0)


def test_percentile_guards() -> None:
    with pytest.raises(ValueError):
        percentile([], 0.5)
    with pytest.raises(ValueError):
        percentile([1.0, 2.0], 1.5)
