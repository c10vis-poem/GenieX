# Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
# SPDX-License-Identifier: BSD-3-Clause

"""Test-model matrix, loaded from ``tests/models.json`` (or ``GENIEX_TEST_MODELS``)."""

from __future__ import annotations

import json
import os
from dataclasses import dataclass
from pathlib import Path

import pytest


@dataclass(frozen=True)
class TestModel:
    role: str
    id: str
    precision: str | None
    hub: str
    devices: tuple[str, ...]
    quality_max_new_tokens: int | None
    nightly: bool

    @property
    def short_name(self) -> str:
        return self.id.rsplit('/', 1)[-1]


def _manifest_path() -> Path:
    override = os.environ.get('GENIEX_TEST_MODELS')
    return Path(override) if override else Path(__file__).with_name('models.json')


def _load() -> dict[str, tuple[TestModel, ...]]:
    with _manifest_path().open(encoding='utf-8') as f:
        raw = json.load(f)['models']
    roles: dict[str, tuple[TestModel, ...]] = {}
    for role, entries in raw.items():
        models = []
        for entry in entries:
            env = entry.get('env_override')
            override = os.environ.get(env) if env else None
            models.append(
                TestModel(
                    role=role,
                    id=override or entry['id'],
                    precision=entry.get('precision'),
                    hub=entry.get('hub', 'auto'),
                    devices=tuple(entry['devices']),
                    quality_max_new_tokens=entry.get('quality_max_new_tokens'),
                    nightly=entry.get('nightly', False),
                )
            )
        roles[role] = tuple(models)
    return roles


MODELS = _load()


def primary(role: str) -> TestModel:
    return MODELS[role][0]


def _marks(model: TestModel) -> tuple:
    return (pytest.mark.nightly,) if model.nightly else ()


def matrix(role: str) -> list:
    return [
        pytest.param(model, device, id=f'{model.short_name}-{device}', marks=_marks(model))
        for model in MODELS[role]
        for device in model.devices
    ]


def pull_cells(*roles: str) -> list:
    return [pytest.param(model, id=model.short_name, marks=_marks(model)) for role in roles for model in MODELS[role]]
