SHELL := /bin/bash

.PHONY: help doctor doctor-summary doctor-strict contracts precommit hook hook-strict

help:
	@echo "FlowForge developer shortcuts:"
	@echo "  make doctor         - run tooling diagnostics (warn profile)"
	@echo "  make doctor-summary - run tooling diagnostics and write summary report"
	@echo "  make doctor-strict  - run tooling diagnostics (strict profile)"
	@echo "  make contracts      - run contract test scripts"
	@echo "  make precommit      - run local pre-commit checks"
	@echo "  make hook           - install managed pre-commit hook"
	@echo "  make hook-strict    - install strict managed pre-commit hook"

doctor:
	./scripts/tooling_doctor.sh

doctor-summary:
	mkdir -p pilot_artifacts/tooling
	./scripts/tooling_doctor.sh --summary-file pilot_artifacts/tooling/latest.tsv
	@echo "Summary: pilot_artifacts/tooling/latest.tsv"

doctor-strict:
	./scripts/tooling_doctor.sh --strict

contracts:
	./scripts/tooling_doctor_contract_test.sh
	./scripts/release_checkpoint_contract_test.sh
	./scripts/install_git_hook_contract_test.sh

precommit:
	./scripts/precommit_checks.sh

hook:
	./scripts/install_git_hook.sh

hook-strict:
	./scripts/install_git_hook.sh --strict
