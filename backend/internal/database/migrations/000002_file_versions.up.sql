CREATE TABLE file_versions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    file_id BIGINT UNSIGNED NOT NULL,
    physical_file_id BIGINT UNSIGNED NOT NULL,
    source VARCHAR(16) NOT NULL,
    restored_from_version_id BIGINT UNSIGNED NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_file_versions_file_time (file_id, created_at, id),
    KEY idx_file_versions_physical_file_id (physical_file_id),
    KEY idx_file_versions_restored_from (restored_from_version_id),
    CONSTRAINT fk_file_versions_file FOREIGN KEY (file_id) REFERENCES files(id) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT fk_file_versions_physical FOREIGN KEY (physical_file_id) REFERENCES physical_files(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_file_versions_restored_from FOREIGN KEY (restored_from_version_id) REFERENCES file_versions(id) ON UPDATE CASCADE ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO file_versions (file_id, physical_file_id, source, created_at)
SELECT id, physical_file_id, 'upload', created_at FROM files;
