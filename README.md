# Go SDK

В этом пакете собрано все, что может быть использовано в нескольких проектах.

С одной стороны, пакет навязывает использование некоторых принципов, с другой стороны, эти принципы не так уж плохи.

Главный принцип - использование proto. Это достаточно гибкое решение, так позволяет описать API, но при этом позволяет
оставить поддержку JSON.

Документация доступна по адресу: https://godoc.customapp.tech/pkg/github.com/custom-app/sdk-go/ .

## Development

1. Установить godoc

```shell
go install golang.org/x/tools/cmd/godoc@latest
```

или

```shell
go get golang.org/x/tools/cmd/godoc
```

2. Запустить локальный сервер

```shell
godoc -goroot=$HOME/go/src -http=:6060
```

3. Перейти по
   ссылке [http://localhost:6060/pkg/github.com/custom-app/sdk-go/](http://localhost:6060/pkg/github.com/custom-app/sdk-go/)

## Ключевые концепты

Для понимания этого SDK надо знать некоторые концепции/паттерны

### Provider

Provider - паттерн работы с Third Party API(сторонние API)
. [подробное описание](https://medium.com/swlh/provider-model-in-go-and-why-you-should-use-it-clean-architecture-1d84cfe1b097)
.

Вкратце суть паттерна в том, что мы определяем интерфейс с несколькими методами и далее пишем несколько реализаций.

Как правило, используется mock-реализация(для тестов) и реализация API-клиента(для реальной работы). Однако, если некие
задачи могут быть решены с помощью разных сторонних API, реализаций может быть больше двух.

В провайдерах данного SDK также предусмотрен провайдер по умолчанию. Суть в том, что в пакете лежит глобальная приватная
переменная, имеющая тип интерфейс провайдера. В пакете имеется метод изменения провайдера по умолчанию и методы,
дублирующие методы из интерфейса, вызывающие эти самые методы у провайдера по умолчанию.