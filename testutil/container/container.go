// Package container - вспомогательный пакет для создания тестовой базы данных в контейнере.
//
// Для работы контейнер PostgreSQL создается отдельно пользователем
package container

import (
	"database/sql"
	"fmt"
	"github.com/custom-app/sdk-go/logger"
	_ "github.com/jackc/pgx/v4/stdlib"
	"time"
)

var (
	rootDb *sql.DB
)

// StartDatabase - создание базы данных в контейнере с именем containerName. указывается пользователь, пароль,
// префикс имени базы данных, к которому будет добавлен unix-timestamp в миллисекундах и порт
func StartDatabase(rootUser, rootPassword, rootDbName, user, pass, databasePrefix string, port int) (*sql.DB, string, error) {
	now := time.Now().UnixNano() / 1e+6
	var err error
	rootDb, err = sql.Open("pgx", fmt.Sprintf("host=localhost port=%d user=%s password=%s dbname=%s sslmode=disable",
		port, rootUser, rootPassword, rootDbName))
	if err != nil {
		return nil, "", err
	}
	databaseName := fmt.Sprintf("%s_%d", databasePrefix, now)
	if _, err := rootDb.Exec(fmt.Sprintf("create database %s", databaseName)); err != nil {
		return nil, "", err
	}
	if _, err := rootDb.Exec(fmt.Sprintf("do $$ BEGIN CREATE USER %s SUPERUSER PASSWORD '%s';"+
		"  EXCEPTION WHEN DUPLICATE_OBJECT THEN RAISE NOTICE 'not creating user'; END $$", user, pass)); err != nil {
		return nil, "", err
	}
	if _, err := rootDb.Exec(fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s", databaseName, user)); err != nil {
		return nil, "", err
	}

	connectionString := fmt.Sprintf("host=localhost port=%d user=%s password=%s dbname=%s sslmode=disable",
		port, user, pass, databaseName)
	db, err := sql.Open("pgx", connectionString)
	if err != nil {
		if err := StopDatabase(databaseName); err != nil {
			logger.Log("stop after failed start err5: ", err)
		}
		return nil, "", err
	}
	success, timeout := make(chan bool), time.After(30*time.Second)
	go func() {
		for {
			if err := db.Ping(); err != nil {
				logger.Log("ping failed", err)
				time.Sleep(time.Second)
			} else {
				success <- true
			}
		}
	}()
	select {
	case <-timeout:
		if err := StopDatabase(databaseName); err != nil {
			logger.Log("stop after failed start err6: ", err)
		}
		return nil, "", fmt.Errorf("ping timeout")
	case <-success:
		break
	}
	return db, databaseName, nil
}

// StopDatabase - удаление базы данных
func StopDatabase(dbName string) error {
	if _, err := rootDb.Exec(fmt.Sprintf("drop database %s", dbName)); err != nil {
		return err
	}
	return nil
}
