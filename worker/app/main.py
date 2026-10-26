"""FastAPI-Anwendung des Workers."""

from __future__ import annotations

import logging
import time
from contextlib import asynccontextmanager
from typing import AsyncIterator

from fastapi import FastAPI, HTTPException, Request, status
from fastapi.responses import JSONResponse

from app import __version__
from app.models import CalcRequest, CalcResponse, HealthResponse
from app.processing import Summary, summarize
from app.settings import settings

logger = logging.getLogger("worker")


def configure_logging() -> None:
    """Konfiguriert strukturiertes JSON-Logging ohne externe Abhaengigkeit."""

    handler = logging.StreamHandler()
    handler.setFormatter(
        logging.Formatter('{"time":"%(asctime)s","level":"%(levelname)s","logger":"%(name)s","message":"%(message)s"}')
    )
    root = logging.getLogger()
    root.handlers.clear()
    root.addHandler(handler)
    root.setLevel(getattr(logging, settings.log_level.upper(), logging.INFO))


@asynccontextmanager
async def lifespan(_: FastAPI) -> AsyncIterator[None]:
    """Start-/Stop-Logging fuer saubere Container-Lebenszyklen."""

    configure_logging()
    logger.info('"worker gestartet service=%s envelope=%s version=%s"', settings.service_name, settings.app_env, __version__)
    yield
    logger.info('"worker beendet"')


app = FastAPI(
    title="Enterprise Tool Worker",
    description="Leichtgewichtiger Verarbeitungs-Worker (Aggregation/Berechnung).",
    version=__version__,
    lifespan=lifespan,
)


@app.get("/healthz", response_model=HealthResponse, tags=["system"])
async def health() -> HealthResponse:
    """Liveness-/Readiness-Endpunkt."""

    return HealthResponse(status="ok", service=settings.service_name, version=__version__)


@app.post("/process/calc", response_model=CalcResponse, tags=["processing"])
async def process_calc(payload: CalcRequest) -> CalcResponse:
    """Aggregiert die uebergebenen Werte und liefert Kennzahlen."""

    started: float = time.perf_counter()

    try:
        summary: Summary = summarize(payload.values)
    except ValueError as exc:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail=str(exc)) from exc

    duration_ms: float = round((time.perf_counter() - started) * 1000.0, 3)
    logger.info('"calc ausgefuehrt count=%d duration_ms=%s"', summary.count, duration_ms)

    return CalcResponse(
        operation=payload.operation,
        count=summary.count,
        sum=summary.total,
        mean=summary.mean,
        median=summary.median,
        min=summary.minimum,
        max=summary.maximum,
        stddev=summary.stddev,
        p90=summary.p90,
        duration_ms=duration_ms,
    )


@app.exception_handler(HTTPException)
async def http_exception_handler(_: Request, exc: HTTPException) -> JSONResponse:
    """Einheitliches Fehlerformat (analog zum Go-Backend)."""

    return JSONResponse(
        status_code=exc.status_code,
        content={"error": {"code": "worker_error", "message": str(exc.detail)}},
    )
