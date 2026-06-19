-- 1. Пользователи
CREATE TABLE IF NOT EXISTS users (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    email         VARCHAR(255)    NOT NULL,
    name          VARCHAR(255)    NOT NULL,
    password_hash VARCHAR(255)    NOT NULL,
    created_at    TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 2. Команды
CREATE TABLE IF NOT EXISTS teams (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name       VARCHAR(255)    NOT NULL,
    created_by BIGINT UNSIGNED NOT NULL,
    created_at TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_teams_created_by FOREIGN KEY (created_by) REFERENCES users (id),
    KEY idx_teams_created_by (created_by)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 3. Связь пользователь ↔ команда (многие-ко-многим) + роль
CREATE TABLE IF NOT EXISTS team_members (
    team_id   BIGINT UNSIGNED                       NOT NULL,
    user_id   BIGINT UNSIGNED                       NOT NULL,
    role      ENUM('owner', 'admin', 'member')      NOT NULL DEFAULT 'member',
    joined_at TIMESTAMP                             NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (team_id, user_id),
    CONSTRAINT fk_tm_team FOREIGN KEY (team_id) REFERENCES teams (id) ON DELETE CASCADE,
    CONSTRAINT fk_tm_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    KEY idx_tm_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 4. Задачи
CREATE TABLE IF NOT EXISTS tasks (
    id          BIGINT UNSIGNED                              NOT NULL AUTO_INCREMENT,
    team_id     BIGINT UNSIGNED                              NOT NULL,
    title       VARCHAR(255)                                 NOT NULL,
    description TEXT                                         NOT NULL,
    status      ENUM('todo', 'in_progress', 'done')          NOT NULL DEFAULT 'todo',
    assignee_id BIGINT UNSIGNED                              NULL,
    created_by  BIGINT UNSIGNED                              NOT NULL,
    created_at  TIMESTAMP                                    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP                                    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_tasks_team     FOREIGN KEY (team_id)     REFERENCES teams (id) ON DELETE CASCADE,
    CONSTRAINT fk_tasks_assignee FOREIGN KEY (assignee_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT fk_tasks_creator  FOREIGN KEY (created_by)  REFERENCES users (id),
    -- Композитный индекс под основной сценарий фильтрации списка задач команды.
    KEY idx_tasks_team_status (team_id, status),
    KEY idx_tasks_assignee (assignee_id),
    KEY idx_tasks_created_by (created_by),
    -- Под аналитику "done за 7 дней".
    KEY idx_tasks_status_updated (status, updated_at),
    -- Под аналитику "топ создателей за месяц".
    KEY idx_tasks_team_created_at (team_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 5. История изменений задач (аудит)
CREATE TABLE IF NOT EXISTS task_history (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    task_id    BIGINT UNSIGNED NOT NULL,
    changed_by BIGINT UNSIGNED NOT NULL,
    field      VARCHAR(64)     NOT NULL,
    old_value  TEXT            NOT NULL,
    new_value  TEXT            NOT NULL,
    changed_at TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_history_task FOREIGN KEY (task_id)    REFERENCES tasks (id) ON DELETE CASCADE,
    CONSTRAINT fk_history_user FOREIGN KEY (changed_by) REFERENCES users (id),
    KEY idx_history_task (task_id),
    KEY idx_history_changed_by (changed_by)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 6. Комментарии к задачам
CREATE TABLE IF NOT EXISTS task_comments (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    task_id    BIGINT UNSIGNED NOT NULL,
    user_id    BIGINT UNSIGNED NOT NULL,
    body       TEXT            NOT NULL,
    created_at TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_comments_task FOREIGN KEY (task_id) REFERENCES tasks (id) ON DELETE CASCADE,
    CONSTRAINT fk_comments_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    KEY idx_comments_task (task_id),
    KEY idx_comments_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
