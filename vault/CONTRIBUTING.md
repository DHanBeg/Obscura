# Contributing to Claudesidian

Thank you for your interest in contributing to claudesidian! This document
provides guidelines for contributing to the project.

## Development Setup

1. Fork the repository
2. Clone your fork: `git clone https://github.com/yourusername/claudesidian.git`
3. Install dependencies: `pnpm install`
4. Create a feature branch: `git checkout -b feature/your-feature-name`

## Commit Message Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/) for clear
commit history:

- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `style:` Code style changes (formatting, etc.)
- `refactor:` Code refactoring
- `test:` Test additions or changes
- `chore:` Maintenance tasks

Examples:

```
feat: add new research-assistant command
fix: correct attachment link updates in scripts
docs: update README with MCP setup instructions
```

## Versioning

We use [Semantic Versioning](https://semver.org/):

- MAJOR (1.0.0): Breaking changes
- MINOR (0.1.0): New features (backward compatible)
- PATCH (0.0.1): Bug fixes (backward compatible)

## Pull Request Process

1. Update the CHANGELOG.md with your changes under "Unreleased"
2. Update documentation if needed
3. Ensure all scripts still work
4. Submit PR with clear description of changes

## Changelog Updates

When contributing, add your changes to CHANGELOG.md under the "Unreleased"
section:

```markdown
## [Unreleased]

### Added

- Your new feature here

### Fixed

- Your bug fix here
```

Use these categories:

- **Added** - New features
- **Changed** - Changes to existing functionality
- **Deprecated** - Features to be removed
- **Removed** - Removed features
- **Fixed** - Bug fixes
- **Security** - Security updates

## Release Process (Maintainers)

1. Update version in package.json
2. Move "Unreleased" items to new version in CHANGELOG.md
3. Commit: `git commit -m "chore: release v0.2.0"`
4. Tag: `git tag v0.2.0`
5. Push: `git push && git push --tags`
6. Create GitHub Release from tag, using changelog content

## Code Style

- Use clear, descriptive variable names
- Comment complex logic
- Keep functions focused and small
- For Bash scripts, use portable shebangs like `#!/usr/bin/env bash` instead of hardcoding `/bin/bash`
- Test your changes thoroughly

## Questions?

Feel free to open an issue for discussion before making large changes.
