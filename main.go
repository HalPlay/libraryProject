package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "modernc.org/sqlite" // Используем Pure Go драйвер
)

// --- Модели данных ---

type Book struct {
	ID     string `json:"id" db:"id"`
	Title  string `json:"title" db:"title"`
	Author string `json:"author" db:"author"`
	ISBN   string `json:"isbn" db:"isbn"`
	Year   int    `json:"year" db:"year"`
	Status string `json:"status" db:"status"`
}

type User struct {
	ID               string    `json:"id" db:"id"`
	Name             string    `json:"name" db:"name"`
	Email            string    `json:"email" db:"email"`
	RegistrationDate time.Time `json:"registration_date" db:"registration_date"`
	Role             string    `json:"role" db:"role"`
}

type Issue struct {
	ID         string       `json:"id" db:"id"`
	BookID     string       `json:"book_id" db:"book_id"`
	UserID     string       `json:"user_id" db:"user_id"`
	IssueDate  time.Time    `json:"issue_date" db:"issue_date"`
	DueDate    time.Time    `json:"due_date" db:"due_date"`
	ReturnDate sql.NullTime `json:"return_date" db:"return_date"`
}

var db *sqlx.DB
var logger *slog.Logger
var jwtSecret []byte

func main() {
	// Загружаем переменные окружения
	_ = godotenv.Load()
	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("default-secret-key")
	}

	// Настройка логирования в JSON
	logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Инициализация БД (используем драйвер "sqlite")
	var err error
	db, err = sqlx.Connect("sqlite", os.Getenv("DB_PATH"))
	if err != nil {
		logger.Error("Failed to connect to DB", "error", err)
		os.Exit(1)
	}
	initSchema()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Роут для получения токена (логин)
	r.Post("/login", loginHandler)

	// Группа защищенных роутов
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)

		r.Get("/books", getBooks)
		r.Get("/books/{id}", getBook)

		// Роуты только для админа
		r.Group(func(r chi.Router) {
			r.Use(adminMiddleware)
			r.Post("/books", createBook)
			r.Put("/books/{id}", updateBook)
			r.Post("/users", createUser)
			r.Post("/issues", issueBook)
			r.Post("/returns", returnBook)
		})

		r.Get("/users/{id}/books", getUserBooks)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info("Server started", "port", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		logger.Error("Server failed", "error", err)
	}
}

// --- Инициализация таблиц ---

func initSchema() {
	schema := `
	CREATE TABLE IF NOT EXISTS books (
		id TEXT PRIMARY KEY,
		title TEXT,
		author TEXT,
		isbn TEXT,
		year INTEGER,
		status TEXT DEFAULT 'Available'
	);
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		name TEXT,
		email TEXT,
		registration_date DATETIME,
		role TEXT DEFAULT 'user'
	);
	CREATE TABLE IF NOT EXISTS issues (
		id TEXT PRIMARY KEY,
		book_id TEXT REFERENCES books(id),
		user_id TEXT REFERENCES users(id),
		issue_date DATETIME,
		due_date DATETIME,
		return_date DATETIME
	);`
	db.MustExec(schema)
}

// --- Middleware (Аутентификация и Роли) ---

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.Header.Get("Authorization")
		if tokenStr == "" {
			http.Error(w, "Missing token", http.StatusUnauthorized)
			return
		}

		claims := jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		r.Header.Set("X-User-Role", fmt.Sprintf("%v", claims["role"]))
		next.ServeHTTP(w, r)
	})
}

func adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-User-Role") != "admin" {
			http.Error(w, "Forbidden: Admins only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Хендлеры ---

func loginHandler(w http.ResponseWriter, r *http.Request) {
	role := r.URL.Query().Get("role")
	if role == "" {
		role = "user"
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"role": role,
		"exp":  time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, _ := token.SignedString(jwtSecret)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": tokenString})
}

func createBook(w http.ResponseWriter, r *http.Request) {
	var b Book
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	b.ID = uuid.New().String()
	b.Status = "Available"

	_, err := db.NamedExec(`INSERT INTO books (id, title, author, isbn, year, status) 
		VALUES (:id, :title, :author, :isbn, :year, :status)`, b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(b)
}

func getBooks(w http.ResponseWriter, r *http.Request) {
	author := r.URL.Query().Get("author")
	status := r.URL.Query().Get("status")

	query := "SELECT * FROM books WHERE 1=1"
	if author != "" {
		query += fmt.Sprintf(" AND author='%s'", author)
	}
	if status != "" {
		query += fmt.Sprintf(" AND status='%s'", status)
	}

	var books []Book
	if err := db.Select(&books, query); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(books)
}

func getBook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var b Book
	if err := db.Get(&b, "SELECT * FROM books WHERE id=$1", id); err != nil {
		http.Error(w, "Book not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(b)
}

func updateBook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var b Book
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	b.ID = id
	_, err := db.NamedExec(`UPDATE books SET title=:title, author=:author, isbn=:isbn, year=:year WHERE id=:id`, b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	u.ID = uuid.New().String()
	u.RegistrationDate = time.Now()
	if u.Role == "" {
		u.Role = "user"
	}

	_, err := db.NamedExec(`INSERT INTO users (id, name, email, registration_date, role) 
		VALUES (:id, :name, :email, :registration_date, :role)`, u)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(u)
}

func issueBook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BookID string `json:"book_id"`
		UserID string `json:"user_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var b Book
	err := db.Get(&b, "SELECT * FROM books WHERE id=$1", req.BookID)
	if err != nil || b.Status != "Available" {
		http.Error(w, "Book is not available", http.StatusConflict)
		return
	}

	tx := db.MustBegin()
	issueID := uuid.New().String()
	now := time.Now()
	dueDate := now.AddDate(0, 0, 14)

	tx.MustExec("INSERT INTO issues (id, book_id, user_id, issue_date, due_date) VALUES ($1, $2, $3, $4, $5)",
		issueID, req.BookID, req.UserID, now, dueDate)
	tx.MustExec("UPDATE books SET status='Issued' WHERE id=$1", req.BookID)

	if err := tx.Commit(); err != nil {
		http.Error(w, "Transaction failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func returnBook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BookID string `json:"book_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	tx := db.MustBegin()
	tx.MustExec("UPDATE issues SET return_date=$1 WHERE book_id=$2 AND return_date IS NULL", time.Now(), req.BookID)
	tx.MustExec("UPDATE books SET status='Available' WHERE id=$1", req.BookID)

	if err := tx.Commit(); err != nil {
		http.Error(w, "Transaction failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func getUserBooks(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var books []Book
	err := db.Select(&books, `SELECT b.* FROM books b JOIN issues i ON b.id = i.book_id 
		WHERE i.user_id = $1 AND i.return_date IS NULL`, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(books)
}
