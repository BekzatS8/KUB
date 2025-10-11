# TurCompany

## 📂 Project Structure

```
turcompany/
├── cmd/
│   ├── bot/
│   │   └── main.go                # Telegram bot entry point
│   └── web/
│       └── main.go                # Web server entry point
├── config/
│   └── config.yaml                # Application configuration
├── db/
│   └── migrations/                # Database migration files
├── docs/                          # Swagger and documentation
├── internal/
│   ├── app/                       # Application bootstrap
│   ├── config/                    # Config loader
│   ├── handlers/                  # HTTP request handlers
│   ├── messaging/                 # Chat bot logic
│   ├── middleware/                # Gin middlewares
│   ├── models/                    # Data models
│   ├── pdf/                       # PDF generation
│   ├── repositories/              # Data access layer
│   ├── routes/                    # Route definitions
│   ├── services/                  # Business logic
│   └── utils/                     # Utility functions
├── go.mod                         # Go module definition
├── go.sum                         # Go dependencies
└── README.md                      # Project documentation
```

---

## 🚀 Features

- **Clean Architecture**: Well-structured layers for separation of concerns.
- **Go 1.23**: Latest version of Go with performance improvements.
- **Scalability**: Designed to handle complex business logic with ease.
- **Config Management**: Centralized and manageable application settings.
- **Database Migrations**: Organized schema changes under the `db/migrations/` directory.

---

## 🛠️ Installation

1. **Clone the repository**:
   ```bash
   git clone https://github.com/shakeniha/turcompany.git
   cd turcompany
   ```

2. **Install dependencies**:
   ```bash
   go mod download
   ```

3. **Set up environment**:
   - Update `config/config.yaml` with your application settings.

4. **Run the application**:
   ```bash
   go run cmd/web/main.go
   ```

---

## 🔧 Configuration

The configuration file (`config/config.yaml`) includes parameters like database credentials, server ports, etc. Modify it to suit your environment.

---

## 🗃️ Database Migrations

Use a tool like [golang-migrate](https://github.com/golang-migrate/migrate) to manage your database migrations. Place your `.sql` files in the `db/migrations/` folder.

To run migrations:
```bash
migrate -path db/migrations -database "your-database-url" up
```

---

## 🤝 Contributing

Contributions are welcome! Please follow these steps:
1. Fork the repository.
2. Create a feature branch.
3. Commit your changes.
4. Open a pull request.


---

## 👨‍💻 Author

- **Alim Zhumabekov** - [ZhumabekovAlim](https://github.com/ZhumabekovAlim)

