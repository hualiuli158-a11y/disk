CREATE TABLE users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    username VARCHAR(100) NOT NULL,
    create_time DATETIME(3) NOT NULL,
    password VARCHAR(255) NOT NULL,
    used_quota BIGINT NOT NULL DEFAULT 0,
    quota_limit BIGINT NOT NULL DEFAULT 10737418240,
    PRIMARY KEY (id),
    UNIQUE KEY idx_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE physical_files (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    sha256 CHAR(64) NOT NULL,
    size BIGINT NOT NULL,
    storage_key VARCHAR(500) NOT NULL,
    thumbnail_key VARCHAR(500) NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY idx_physical_files_sha256 (sha256),
    UNIQUE KEY idx_physical_files_storage_key (storage_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE folders (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    parent_id BIGINT UNSIGNED NULL,
    name VARCHAR(255) NOT NULL,
    deletion_key VARCHAR(64) NOT NULL DEFAULT '',
    trash_id BIGINT UNSIGNED NULL,
    deleted_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY idx_active_folder_name (user_id, parent_id, name, deletion_key),
    KEY idx_folders_parent_id (parent_id),
    KEY idx_folders_trash_id (trash_id),
    KEY idx_folders_deleted_at (deleted_at),
    CONSTRAINT fk_folders_user FOREIGN KEY (user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT fk_folders_parent FOREIGN KEY (parent_id) REFERENCES folders(id) ON UPDATE CASCADE ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE files (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    folder_id BIGINT UNSIGNED NOT NULL,
    physical_file_id BIGINT UNSIGNED NOT NULL,
    name VARCHAR(255) NOT NULL,
    deletion_key VARCHAR(64) NOT NULL DEFAULT '',
    trash_id BIGINT UNSIGNED NULL,
    deleted_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY idx_active_file_name (user_id, folder_id, name, deletion_key),
    KEY idx_files_folder_id (folder_id),
    KEY idx_files_physical_file_id (physical_file_id),
    KEY idx_files_trash_id (trash_id),
    KEY idx_files_deleted_at (deleted_at),
    CONSTRAINT fk_files_user FOREIGN KEY (user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT fk_files_folder FOREIGN KEY (folder_id) REFERENCES folders(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_files_physical FOREIGN KEY (physical_file_id) REFERENCES physical_files(id) ON UPDATE CASCADE ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE trash_entries (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    resource_type VARCHAR(16) NOT NULL,
    resource_id BIGINT UNSIGNED NOT NULL,
    original_parent_id BIGINT UNSIGNED NULL,
    name VARCHAR(255) NOT NULL,
    trashed_at DATETIME(3) NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_trash_entries_user_id (user_id),
    KEY idx_trash_entries_resource_type (resource_type),
    KEY idx_trash_entries_resource_id (resource_id),
    KEY idx_trash_entries_original_parent_id (original_parent_id),
    KEY idx_trash_entries_trashed_at (trashed_at),
    KEY idx_trash_entries_expires_at (expires_at),
    CONSTRAINT fk_trash_entries_user FOREIGN KEY (user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
