---
type: Reference
title: Architecture Overview
description: The major components of the service and how they fit together.
tags: [architecture]
---

# Architecture Overview

The service is a small CLI over a set of internal packages. New engineers should
skim this before their first change; see the [onboarding
playbook](/topics/onboarding.md) for the surrounding process.

## Components

- **cmd** — the command surface.
- **internal** — the domain logic.
