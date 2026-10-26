"""API-Tests des Workers (ohne laufenden Netzwerk-Stack)."""

from __future__ import annotations

from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)


def test_healthz() -> None:
    response = client.get("/healthz")

    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "ok"
    assert body["service"] == "anchor-blueprint-worker"


def test_process_calc_returns_aggregates() -> None:
    payload = {"values": [120, 340, 95, 480, 210, 365, 155, 290]}

    response = client.post("/process/calc", json=payload)

    assert response.status_code == 200
    body = response.json()
    assert body["operation"] == "summary"
    assert body["count"] == 8
    assert abs(body["sum"] - 2055.0) < 1e-6
    assert abs(body["mean"] - 256.875) < 1e-6
    assert abs(body["median"] - 250.0) < 1e-6
    assert abs(body["min"] - 95.0) < 1e-6
    assert abs(body["max"] - 480.0) < 1e-6
    assert abs(body["p90"] - 399.5) < 1e-6
    assert body["duration_ms"] >= 0.0


def test_process_calc_rejects_empty_values() -> None:
    response = client.post("/process/calc", json={"values": []})

    assert response.status_code == 422


def test_process_calc_rejects_unknown_fields() -> None:
    response = client.post("/process/calc", json={"values": [1, 2], "extra": "x"})

    assert response.status_code == 422


def test_process_calc_rejects_too_many_values() -> None:
    response = client.post("/process/calc", json={"values": [1.0] * 10001})

    assert response.status_code == 422
