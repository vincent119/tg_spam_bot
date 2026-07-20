## ADDED Requirements

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

- **WHEN** `log.max_files` is greater than zero and `log.rotate.max_backups` is unset or zero
- **THEN** the system can map `log.max_files` to the effective rotation backup count

#### Scenario: New rotation backup count provided

- **WHEN** both `log.max_files` and `log.rotate.max_backups` are greater than zero
- **THEN** the system uses `log.rotate.max_backups` as the effective rotation backup count

### Requirement: Documentation for file log rotation

The system SHALL document the file log rotation configuration in sample config and README.

#### Scenario: Sample config includes rotation

- **WHEN** a user reads `configs/config.sample.yaml`
- **THEN** the sample shows `log.rotate` fields and explains that rotation only applies when `log.outputs` contains `file`

#### Scenario: README explains log semantics

- **WHEN** a user reads README log configuration documentation
- **THEN** the documentation distinguishes `log.format`, `log.outputs`, and `log.rotate`
