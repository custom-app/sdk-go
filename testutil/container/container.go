// Package container - вспомогательный пакет для создания тестовой базы данных в контейнере.
//
// Для работы контейнер PostgreSQL создается отдельно пользователем
package container

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/loyal-inform/sdk-go/logger"
	"io/ioutil"
	"time"
)

func findDatabase(cli *client.Client, containerName string) (*types.Container, error) {
	list, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	for _, c := range list {
		for _, n := range c.Names {
			if n == containerName {
				return &c, nil
			}
		}
	}
	return nil, fmt.Errorf("not found")
}

type execResult struct {
	StdOut   string
	StdErr   string
	ExitCode int
}

func exec(cli *client.Client, containerID string, command []string) (types.IDResponse, error) {
	config := types.ExecConfig{
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          command,
	}

	return cli.ContainerExecCreate(context.Background(), containerID, config)
}

func inspectExecResp(cli *client.Client, id string) (execResult, error) {
	var execResult execResult

	resp, err := cli.ContainerExecAttach(context.Background(), id, types.ExecConfig{})
	if err != nil {
		return execResult, err
	}
	defer resp.Close()

	var outBuf, errBuf bytes.Buffer
	outputDone := make(chan error)

	go func() {
		_, err = stdcopy.StdCopy(&outBuf, &errBuf, resp.Reader)
		outputDone <- err
	}()

	select {
	case err := <-outputDone:
		if err != nil {
			return execResult, err
		}
		break
	}

	stdout, err := ioutil.ReadAll(&outBuf)
	if err != nil {
		return execResult, err
	}
	stderr, err := ioutil.ReadAll(&errBuf)
	if err != nil {
		return execResult, err
	}

	res, err := cli.ContainerExecInspect(context.Background(), id)
	if err != nil {
		return execResult, err
	}

	execResult.ExitCode = res.ExitCode
	execResult.StdOut = string(stdout)
	execResult.StdErr = string(stderr)
	return execResult, nil
}

// StartDatabase - создание базы данных в контейнере с именем containerName. указывается пользователь, пароль,
// префикс имени базы данных, к которому будет добавлен unix-timestamp в миллисекундах и порт
func StartDatabase(containerName, user, pass, databasePrefix string, port int) (*sql.DB, string, error) {
	now := time.Now().UnixNano() / 1e+6
	c, err := client.NewEnvClient()
	if err != nil {
		return nil, "", err
	}
	cont, err := findDatabase(c, containerName)
	if err != nil {
		return nil, "", err
	}

	databaseName := fmt.Sprintf("%s_%d", databasePrefix, now)
	logger.Log("creating database", databaseName)
	id, err := exec(c, cont.ID, []string{
		"psql",
		"-U",
		"postgres",
		"-c",
		fmt.Sprintf("create database %s;", databaseName),
	})
	if err != nil {
		return nil, "", err
	}
	res, err := inspectExecResp(c, id.ID)
	if err != nil {
		return nil, "", err
	}
	logger.Log("first exec stdout: ", res.StdOut)
	logger.Log("first exec stderr: ", res.StdErr)
	if res.ExitCode != 0 {
		return nil, "", fmt.Errorf("first exec failed: %d", res.ExitCode)
	}
	id, err = exec(c, cont.ID, []string{
		"psql",
		"-U",
		"postgres",
		"-c",
		fmt.Sprintf("DO $$\nBEGIN\nCREATE USER %s SUPERUSER PASSWORD '%s';\n"+
			"EXCEPTION WHEN DUPLICATE_OBJECT THEN\nRAISE NOTICE 'not creating user';\nEND\n$$;\n"+
			"GRANT ALL PRIVILEGES ON DATABASE %s TO %s;\n", user, pass, databaseName, user),
	})
	if err != nil {
		if err := StopDatabase(databaseName, containerName); err != nil {
			logger.Log("stop after failed start err12: ", err)
		}
		return nil, "", err
	}
	res, err = inspectExecResp(c, id.ID)
	if err != nil {
		if err := StopDatabase(databaseName, containerName); err != nil {
			logger.Log("stop after failed start err: ", err)
		}
		return nil, "", err
	}
	logger.Log("second exec stdout: ", res.StdOut)
	logger.Log("second exec stderr: ", res.StdErr)
	if res.ExitCode != 0 {
		if err := StopDatabase(databaseName, containerName); err != nil {
			logger.Log("stop after failed start err3: ", err)
		}
		return nil, "", fmt.Errorf("second exec failed: %d", res.ExitCode)
	}

	if err := c.Close(); err != nil {
		if err := StopDatabase(databaseName, containerName); err != nil {
			logger.Log("stop after failed start err4: ", err)
		}
		return nil, "", err
	}
	connectionString := fmt.Sprintf("host=localhost port=%d user=%s password=%s dbname=%s sslmode=disable",
		port, user, pass, databaseName)
	db, err := sql.Open("pgx", connectionString)
	if err != nil {
		if err := StopDatabase(databaseName, containerName); err != nil {
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
		if err := StopDatabase(databaseName, containerName); err != nil {
			logger.Log("stop after failed start err6: ", err)
		}
		return nil, "", fmt.Errorf("ping timeout")
	case <-success:
		break
	}
	return db, databaseName, nil
}

// StopDatabase - удаление базы данных
func StopDatabase(containerName, d string) error {
	c, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	cont, err := findDatabase(c, containerName)
	if err != nil {
		return err
	}
	id, err := exec(c, cont.ID, []string{
		"psql",
		"-U",
		"postgres",
		"-c",
		fmt.Sprintf("drop database %s;\n", d),
	})
	if err != nil {
		return err
	}
	res, err := inspectExecResp(c, id.ID)
	if err != nil {
		return err
	}
	logger.Log("drop exec stdout: ", res.StdOut)
	logger.Log("drop exec stderr: ", res.StdErr)
	if res.ExitCode != 0 {
		return fmt.Errorf("drop exec failed: %d", res.ExitCode)
	}
	return c.Close()
}
