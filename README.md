/*
=========================================================================================
ИНСТРУКЦИЯ ДЛЯ ТЕСТОВ (КОПИРУЙ И ВСТАВЛЯЙ В POWERSHELL)
=========================================================================================

1. ЗАПУСК СЕРВЕРА (из корня папки library):
go run cmd/server/main.go

2. ПОЛУЧЕНИЕ ТОКЕНА (ЛОГИН АДМИНА):
curl.exe -X POST "http://localhost:8080/login?role=admin"

3. СОХРАНЕНИЕ ТОКЕНА (вставь токен из ответа выше вместо "..."):
$t = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVC..."

4. ДОБАВЛЕНИЕ КНИГИ:
$b = '{"title": "Gopher Guide", "author": "Rob Pike", "isbn": "123-456", "year": 2023}'
Invoke-RestMethod -Method Post -Uri "http://localhost:8080/books" -Headers @{"Authorization"=$t} -ContentType "application/json" -Body $b

5. СОЗДАТЬ ЧИТАТЕЛЯ:
$u = '{"name": "Ivan Ivanov", "email": "ivan@example.com"}'
Invoke-RestMethod -Method Post -Uri "http://localhost:8080/users" -Headers @{"Authorization"=$t} -ContentType "application/json" -Body $u

6. ВЫДАТЬ КНИГУ (вставьте ID книги и юзера из прошлых ответов):
$iss = '{"book_id": "ID_КНИГИ", "user_id": "ID_ЮЗЕРА"}'
Invoke-RestMethod -Method Post -Uri "http://localhost:8080/issues" -Headers @{"Authorization"=$t} -ContentType "application/json" -Body $iss

7. ПОСМОТРЕТЬ РЕЗУЛЬТАТ (статус должен стать "Issued"):
Invoke-RestMethod -Method Get -Uri "http://localhost:8080/books" -Headers @{"Authorization"=$t} | ConvertTo-Json
=========================================================================================
*/

