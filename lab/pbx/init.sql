-- FreePBX uchun bazalar (lab)
CREATE DATABASE asterisk;
GRANT ALL PRIVILEGES ON `asterisk`.* TO 'freepbxuser'@'%';
CREATE DATABASE asteriskcdrdb;
GRANT ALL PRIVILEGES ON `asteriskcdrdb`.* TO 'freepbxuser'@'%';
FLUSH PRIVILEGES;
