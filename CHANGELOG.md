# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- CI/CD pipeline with automated versioning and releases
- Version endpoint at `/api/v1/version`
- Automated Docker image tagging (version + latest)

### Changed
- Tests now run only on merge requests or manual triggers
- Builds run only on version tags

## [0.1.0] - 2024-XX-XX

### Added
- Initial release
- Entity management system
- Course generation with Marp and Slidev
- Authentication with Casdoor
- Lab environment management
- Terminal trainer integration
- Payment system with Stripe

[unreleased]: https://gitlab.com/your-org/ocf-core/compare/v0.1.0...HEAD
[0.1.0]: https://gitlab.com/your-org/ocf-core/releases/tag/v0.1.0
