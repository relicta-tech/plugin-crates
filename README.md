# Crates Plugin for Relicta

Official Crates plugin for [Relicta](https://github.com/relicta-tech/relicta) - Publish crates to crates.io (Rust).

## Installation

```bash
relicta plugin install crates
relicta plugin enable crates
```

## Configuration

Add to your `release.config.yaml`:

```yaml
plugins:
  - name: crates
    enabled: true
    config:
      # Add configuration options here
```

## License

MIT License - see [LICENSE](LICENSE) for details.
