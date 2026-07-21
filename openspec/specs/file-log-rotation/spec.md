## Purpose

定義檔案日誌輸出與輪轉設定，讓系統在輸出到檔案時能控制單檔大小、保留數量、保存天數與壓縮行為，同時保留舊版 `log.max_files` 的相容處理。

## Requirements

### Requirement: Log format and output destination separation

The system SHALL treat `log.format` as the log encoding format and `log.outputs` as the output destination list.

#### Scenario: JSON format with console output

- **WHEN** `log.format` is `json` and `log.outputs` contains `console`
- **THEN** the system writes JSON encoded logs to console output

#### Scenario: File output destination

- **WHEN** `log.outputs` contains `file`
- **THEN** the system writes logs to `log.path` and `log.file`

#### Scenario: Invalid format value

- **WHEN** `log.format` is neither `json` nor `console`
- **THEN** configuration validation fails during startup

### Requirement: File log rotation configuration

The system SHALL support file log rotation through a `log.rotate` configuration block.

#### Scenario: Rotation enabled

- **WHEN** `log.outputs` contains `file` and `log.rotate.enabled` is `true`
- **THEN** the system uses `log.rotate.max_size_mb`, `log.rotate.max_backups`, `log.rotate.max_age_days`, and `log.rotate.compress` to control file log rotation

#### Scenario: Rotation disabled

- **WHEN** `log.outputs` contains `file` and `log.rotate.enabled` is `false`
- **THEN** the system writes to the configured log file without application-managed rotation

#### Scenario: Console only output

- **WHEN** `log.outputs` does not contain `file`
- **THEN** the system does not require `log.path`, `log.file`, or `log.rotate` to be valid for file writing

#### Scenario: Rotation defaults

- **WHEN** `log.rotate` fields are omitted
- **THEN** the system uses `enabled=false`, `max_size_mb=100`, `max_backups=14`, `max_age_days=30`, and `compress=true` as defaults

#### Scenario: Zero max size

- **WHEN** `log.rotate.enabled` is `true` and `log.rotate.max_size_mb` is `0`
- **THEN** the system uses `100` as the effective max size in megabytes

#### Scenario: Zero backup count

- **WHEN** `log.rotate.enabled` is `true` and `log.rotate.max_backups` is `0`
- **THEN** the system does not limit rotated log files by backup count

#### Scenario: Zero retention days

- **WHEN** `log.rotate.enabled` is `true` and `log.rotate.max_age_days` is `0`
- **THEN** the system does not delete rotated log files by age

### Requirement: Rotation validation

The system SHALL validate log rotation settings during startup.

#### Scenario: Negative rotation size

- **WHEN** `log.rotate.max_size_mb` is negative
- **THEN** configuration validation fails during startup

#### Scenario: Negative backup count

- **WHEN** `log.rotate.max_backups` is negative
- **THEN** configuration validation fails during startup

#### Scenario: Negative retention days

- **WHEN** `log.rotate.max_age_days` is negative
- **THEN** configuration validation fails during startup

#### Scenario: File output requires target path

- **WHEN** `log.outputs` contains `file`
- **THEN** the system has a deterministic log directory and file name before logger initialization

### Requirement: Legacy max files compatibility

The system SHALL keep `log.max_files` as a deprecated compatibility setting during the migration period.

#### Scenario: Legacy max files provided

- **WHEN** `log.rotate.enabled` is `false` and `log.max_files` is greater than zero
- **THEN** the system preserves the existing startup log pruning behavior

#### Scenario: Rotation enabled with legacy max files

- **WHEN** `log.rotate.enabled` is `true`
- **THEN** the system does not execute the legacy startup log pruning behavior

### Requirement: Documentation for file log rotation

The system SHALL document the file log rotation configuration in sample config and README.

#### Scenario: Sample config includes rotation

- **WHEN** a user reads `configs/config.sample.yaml`
- **THEN** the sample shows `log.rotate` fields and explains that rotation only applies when `log.outputs` contains `file`

#### Scenario: README explains log semantics

- **WHEN** a user reads README log configuration documentation
- **THEN** the documentation distinguishes `log.format`, `log.outputs`, and `log.rotate`
