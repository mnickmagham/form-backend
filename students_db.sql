
CREATE DATABASE IF NOT EXISTS students_db
    CHARACTER SET utf8mb4
    COLLATE utf8mb4_unicode_ci;

USE students_db;

CREATE TABLE students (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    student_id VARCHAR(20) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    major VARCHAR(100),
    degree VARCHAR(50),
    phone_number VARCHAR(30)
);
