# umeshctl — руководство по инициализации и эксплуатации ноды Umesh

`umeshctl` — консольная утилита, которая запускается **на хосте** (не внутри
контейнера ноды) и управляет блокчейн-нодой [Umesh](https://github.com/umesh-network)
на **Cosmos SDK 0.54 + CometBFT**. Нода живёт в Docker-контейнере; `umeshctl`
подключается к ней снаружи и берёт на себя всё, что раньше делалось руками:
правку `config.toml`/`app.toml`, генерацию ключей и gentx, скачивание
genesis.json и addrbook, расчёт параметров генезиса, слежение за синхронизацией
и резервное копирование.

Этот документ — переработанная версия `README.md` из ветки `dev`
(`opscores/umesh-cli`). Структура и объяснения выстроены заново так, чтобы
можно было последовательно читать сверху вниз и понимать: что происходит на
каждом шаге, зачем он нужен, какие значения куда вписывать и откуда их брать.
Содержание команд и параметров проверено по исходному коду репозитория
(пакеты `cmd/`, `internal/nodeinit`, `internal/tune`, `internal/yamlconfig`,
`internal/backup`).

> **Важно для практики:** `ops backup` **не** копирует каталог `keyring/`
> (только `priv_validator_key.json`, `node_key.json`, `genesis.json`,
> `priv_validator_state.json`) — бэкапьте его отдельно. А Docker-образ `umesh-node`
> и `docker-compose.yml` живут **в отдельном репозитории `Node_Umesh`**, а не в
> `umesh-cli`.


## Содержание

1. [Как устроена нода и при чём тут umeshctl](#1-как-устроена-нода-и-при-чём-тут-umeshctl)
2. [Требования и установка](#2-требования-и-установка)
3. [Быстрый старт](#3-быстрый-старт)
4. [Как вообще работает umeshctl: офлайн- и онлайн-операции](#4-как-вообще-работает-umeshctl-офлайн--и-онлайн-операции)
5. [Конфигурация и секреты](#5-конфигурация-и-секреты)
6. [Сценарий Genesis: создание сети с нуля](#6-сценарий-genesis-создание-сети-с-нуля)
7. [Сценарий Validator: подключение валидатора к живой сети](#7-сценарий-validator-подключение-валидатора-к-живой-сети)
8. [Сценарий Sentry: публичный щит валидатора](#8-сценарий-sentry-публичный-щит-валидатора)
9. [Сценарий RPC: публичная точка входа](#9-сценарий-rpc-публичная-точка-входа)
10. [Эксплуатация после запуска](#10-эксплуатация-после-запуска)
11. [Резервное копирование и восстановление](#11-резервное-копирование-и-восстановление)
12. [Genesis plan (YAML) — подробный разбор](#12-genesis-plan-yaml--подробный-разбор)
13. [Справочник команд](#13-справочник-команд)
14. [Глобальные флаги и переменные окружения](#14-глобальные-флаги-и-переменные-окружения)
15. [Частые ошибки и диагностика](#15-частые-ошибки-и-диагностика)

## 1. Как устроена нода и при чём тут umeshctl

Каждая нода Umesh — это Docker-контейнер с бинарником `umeshd`. Внутри
контейнера есть home-каталог (`--home`, по умолчанию `/home/umesh/.umeshd`),
в котором лежат все важные файлы ноды:

| Файл/каталог | За что отвечает |
| ------------ | --------------- |
| `config/genesis.json` | Начальное состояние сети: chain_id, валидаторы, балансы, параметры модулей |
| `config/config.toml` | P2P-настройки: пиры, seeds, RPC, состояние синхронизации |
| `config/app.toml` | Прикладные параметры: API, pruning, min-gas-price |
| `config/node_key.json` | Идентичность ноды в P2P-сети (**NodeID**) |
| `config/priv_validator_key.json` | Ключ подписи блоков валидатора — **самый ценный файл на ноде** |
| `keyring/` | Ключи-аккаунты (file-бэкенд), защищены паролем |
| `config/gentx/` | Заявки валидаторов на вхождение в генезис (gentx) |

Обычно с этими файлами приходится работать руками: заходить в контейнер,
редактировать TOML, генерировать ключи через `umeshd`. `umeshctl` убирает эту
рутину — вы задаёте параметры через **YAML-конфиг** и CLI-флаги, а утилита сама
монтирует хостовый каталог данных (`--data-dir`, по умолчанию для каждой роли
своего: `./data-validator`, `./data-sentry`, `./data-rpc`) в контейнер и выполняет
там нужные команды `umeshd`.

Секреты (пароль keyring) в YAML **никогда** не хранятся — только через CLI секрет-источники:
`--keyring-password-file / --keyring-password-stdin / --keyring-password-exec`,
либо `--auto-password` (автогенерация). Подробнее — в разделе
[«Конфигурация и секреты»](#5-конфигурация-и-секреты).

### Четыре роли ноды

Первое, что нужно решить — какую роль вы разворачиваете. От этого зависит всё
остальное: какие данные готовить, какие порты открывать, нужен ли кошелёк.

| Роль | Когда нужна | Коротко что делает `init <role>` |
| ---- | ----------- | -------------------------------- |
| `genesis` | Вы создаёте новую сеть с нуля (блок 0) | Создаёт home-каталог, ключи, `config.toml`/`app.toml`, подготавливает генезис и gentx |
| `validator` | Вы присоединяетесь к уже существующей сети как валидатор (post-genesis) | Скачивает `genesis.json` у координатора/sentry/другого валидатора, извлекает `bond_denom`, готовит consensus-ключи |
| `sentry` | Вы ставите публичный узел-прокладку между валидатором и остальной сетью | Публичный P2P-узел, скрывающий валидатора от прямых подключений |
| `rpc` | Вы ставите публичный RPC/REST-узел для пользователей, кошельков, эксплореров | Проксирует запросы к сети наружу |

**Важная деталь, которой нет в исходном коде umesh-cli, но которая нужна для
запуска ноды:** Docker-образ `umesh-node:latest` и файлы `Dockerfile` /
`docker-compose.yml` находятся **в отдельном репозитории `Node_Umesh`**, а не
в `umesh-cli`. `umeshctl` — это только CLI для управления уже собранным
образом; сам образ и compose-файлы нужно получить/собрать отдельно (у
координатора проекта Umesh или в соответствующем репозитории). Если у вас
образа `umesh-node:latest` нет — `init` завершится ошибкой вида
`Preflight: image not found`.


## 2. Требования и установка

- **Go 1.25.12** или новее — только для сборки `umeshctl` из исходников.
- **Работающий Docker daemon** — нужен для всех операций, которые трогают ноду
  (инициализация, запуск, диагностика). Для чисто конфигурационных задач
  (например, редактирования YAML-плана) Docker не обязателен.

```bash
git clone git@github.com:opscores/umesh-cli.git
cd umesh-cli
make build            # соберёт бинарник ./umeshctl
```

Либо сразу установить в `$GOPATH/bin`:

```bash
go install ./...
```

Проверка, что всё собралось:

```bash
umeshctl version
umeshctl --help
```


## 3. Быстрый старт

Минимальный сценарий для тех, кто уже примерно понимает Cosmos SDK и просто
хочет увидеть последовательность целиком: инициализировать ноду-валидатор,
убедиться что она стартовала, и сразу сделать бэкап ключей.

```bash
# 1. Инициализация ноды-валидатора (данные лягут в ./data-validator)
umeshctl init validator --config node-config.yaml \
  --keyring-password-file /path/to/keyring-password.txt

# 2. Дальше нужно запустить контейнер средствами docker compose из репозитория
#    Node_Umesh (см. пояснение выше) — umeshctl сам контейнер не поднимает.

# 3. После запуска контейнера — проверяем статус, синхронизацию, пиров
umeshctl node status sync
umeshctl node health
umeshctl node peers list

# 4. Резервная копия ключей и конфигов (контейнер должен быть ЗАПУЩЕН — см. §11)
umeshctl ops backup --output ./backups
```

Если это ваш первый запуск ноды или вы не уверены, какая роль вам нужна —
пропустите быстрый старт и читайте разделы 6–9: там пошагово расписан каждый
сценарий с объяснением, откуда брать значения.


## 4. Как вообще работает umeshctl: офлайн- и онлайн-операции

Это ключевая идея, которую полезно держать в голове на всех дальнейших шагах.
Команды `umeshctl` делятся на два непересекающихся типа:

- **Офлайн-операции** — работают через `docker run --rm -u 1000:1000` с
  bind-mount хостового каталога данных. Контейнер ноды в этот момент **не
  должен быть запущен**. Сюда относятся: `init <role>`, `genesis plan`,
  `setup keys add`, `node keys export/import`, `node prune`,
  `node snapshot create/list/restore`, `ops restore`, `init <role> --force`.
- **Онлайн-операции** — работают через `docker exec` в уже запущенный
  контейнер. Сюда относятся: `node status`, `node health`, `validator create`,
  `sentry connect`, `rpc set-upstream`, `ops backup`, `ops verify`,
  большинство команд `node config`.

Если перепутать фазу, будет одна из двух ошибок:

- офлайн-команда против запущенного контейнера → `refusing --force while
  container is running` (нужно сначала `docker compose down`);
- онлайн-команда против остановленного контейнера → `container "..." is not
  running — start node first`.

Почему это важно, а не просто техническая деталь: `priv_validator_key.json` и
`priv_validator_state.json` нельзя трогать, пока нода работает и подписывает
блоки — восстановление старого состояния поверх текущего может привести к
**double-sign** и permanent jail/tombstone валидатора. Поэтому все операции,
которые пишут в эти файлы (restore, snapshot restore, `--force`), намеренно
требуют остановленного контейнера.


## 5. Конфигурация и секреты

### YAML-конфиг ноды

Все параметры ноды (роль, chain_id, moniker, denom, источники генезиса, пиры и
т.д.) задаются в одном YAML-файле, который передаётся через `--config
node-config.yaml`. Шаблоны лежат в `examples/node-config/{validator,sentry,rpc}.yaml`
— их достаточно скопировать и заполнить своими значениями. Любое поле из YAML
можно переопределить флагом прямо в команде — флаги имеют наивысший приоритет.

Поля, которые **обязательны всегда, независимо от роли**:

| Поле | Описание |
| ---- | -------- |
| `apiVersion` | Должно быть строго `umesh.network/v1` |
| `kind` | Должно быть строго `Node` |
| `role` | `genesis` / `validator` / `sentry` / `rpc` |
| `node.moniker` | Имя ноды — произвольная строка, которую вы придумываете сами |
| `node.environment` | Например, `production` или `testnet` |
| `chain.minGasPrice` | Минимальная цена газа, например `0.0025` |

Поля, зависящие от роли:

| Роль | Дополнительно обязательно | Дополнительно опционально |
| ---- | -------------------------- | -------------------------- |
| `genesis` | `chain.chainId`, `chain.denom` | `validator.*` (параметры генезис-валидатора) |
| `validator` | как минимум один из `join.genesisUrl` / `join.sentryRpc` / `join.validatorRpc` | `chain.chainId`, `chain.denom` — если не указать, возьмутся автоматически из скачанного генезиса; `network.externalAddress`, `network.persistentPeers`/`seeds` |
| `sentry` | как минимум один из `join.*` | `chain.chainId`/`denom` — авто; `network.externalAddress/publicIp/externalPort`, `network.usePrivate` |
| `rpc` | `join.sentryRpc` **или** `join.genesisUrl` (как минимум один) | `chain.chainId`/`denom` — авто; `node.pruning`, `network.*` |

> Секция `join` **не допускается** для роли `genesis` — генезис-нода не
> присоединяется к чужой сети, а создаёт свою; если вписать `join` в YAML с
> `role: genesis`, конфиг не пройдёт валидацию.

### Пароль keyring

Пароль от keyring — единственный секрет, который умешctl вообще просит, и он
**никогда** не должен лежать в YAML: валидатор конфигов блокирует поля с
именами `password`, `secret`, `mnemonic` и подобные. Передавайте его одним из
способов:

| Флаг | Как передаётся |
| ---- | -------- |
| `--keyring-password-file <путь>` | Прочитать пароль из файла |
| `--keyring-password-stdin` | Прочитать пароль из stdin |
| `--keyring-password-exec <команда>` | Выполнить команду и взять пароль из её stdout (например, для интеграции с secret-менеджером) |
| `--auto-password` | Сгенерировать случайный пароль (32 символа из `[A-Za-z0-9]`, криптографически случайные байты) и сохранить его в `~/.config/umesh/keyring.pass` (XDG config dir) с правами `0600` |

`--auto-password` удобен для тестовых/стейджинговых сетей, но помните: пароль
лежит открытым текстом в XDG config dir (`~/.config/umesh/keyring.pass`).
Для продакшен-валидатора разумнее использовать `--keyring-password-file` с
файлом, лежащим отдельно (и включённым в ваш собственный процесс бэкапа
секретов).

### OpenTelemetry (опционально)

Если в окружении задана переменная `OTEL_ENDPOINT`, при выполнении `setup
init` в `config/otel.yaml` пишется конфигурация экспорта трейсов/метрик/логов
по gRPC. **Важно:** Cosmos SDK читает телеметрию именно из этого файла, а не
из переменных `OTEL_EXPORTER_*` — они игнорируются. Файл перезаписывается
только когда `init` реально что-то делает (свежая нода или `--force`);
повторный идемпотентный запуск его не трогает. Без `OTEL_ENDPOINT` файл
остаётся пустым, и телеметрия выключена.


## 6. Сценарий Genesis: создание сети с нуля

Эта роль нужна, если вы — тот, кто создаёт сеть с блока 0. Здесь два пути:
простой (для тестов/разработки) и production (декларативный план).

### 6.1. Простой путь (dev/тест)

```bash
umeshctl init genesis --config node-config.yaml --auto-password
```

Это создаёт home-каталог, генерирует `node_key.json`/`priv_validator_key.json`,
создаёт `config.toml`/`app.toml` и подготавливает базовый генезис под ваши
`chain.chainId`/`chain.denom`/`validator.*` из YAML.

### 6.2. Production-путь: генезис из YAML-плана

Для реальной сети стартовое состояние (`genesis.json`) лучше описывать
декларативным **YAML-планом** — он отвечает на вопросы: сколько всего
токенов, кому и с каким vesting они распределены, какие параметры у модулей,
нужен ли поэтапный (soft) запуск. Подробный разбор структуры плана — в
разделе [12](#12-genesis-plan-yaml--подробный-разбор). Пример лежит в
`examples/genesis-plan.yaml`.

```bash
# 1. Проверить план на корректность, ничего не создавая
umeshctl genesis validate-plan --config examples/genesis-plan.yaml

# 2. Посмотреть, кто и сколько токенов получит (человекочитаемо или JSON)
umeshctl genesis report --config examples/genesis-plan.yaml
umeshctl genesis report --config examples/genesis-plan.yaml --output json

# 3. Фактически создать генезис из плана
umeshctl genesis plan --config examples/genesis-plan.yaml --auto-password
```

Дополнительные флаги `genesis plan`:

| Флаг | Зачем |
| ---- | ----- |
| `--force` | Пересоздать генезис, даже если `genesis.json` уже существует |
| `--force --keep-keys` | Пересоздать состояние сети, но **сохранить** идентичность валидатора (consensus- и P2P-ключи не меняются). Требует `--force` — без него флаг отклоняется |
| `--keyring-password-file` / `--stdin` / `--exec` / `--auto-password` | Как и везде — способ передать пароль keyring |

### 6.3. Координация валидаторов («Mainnet Ritual»)

После того как базовый генезис создан, к сети нужно добавить остальных
валидаторов и их аккаунты, а затем собрать все `gentx` в один документ.

```bash
# Добавить аккаунт (обычный или с vesting)
umeshctl genesis add-account --key-name advisor --type delayed_vesting \
  --amount 50000000000000uumesh --end-time 2027-08-15T00:00:00Z

# Добавить модульный аккаунт (например, для distribution модуля)
umeshctl genesis add-account --key-name distr --type module_account \
  --module-name distribution --amount 1000000uumesh

# Добавить валидатора и сразу сгенерировать для него gentx
umeshctl genesis add-validator --key-name validator-2 --moniker "My Node" \
  --self-delegation 100000000000000uumesh --chain-id umesh-1

# Скачать gentx-файлы других валидаторов из репозитория-координатора и собрать
# их все в единый генезис
umeshctl genesis collect-gentx --repo https://github.com/org/gentx-repo/archive/refs/heads/main.zip
# ⚠️ collect-gentx использует docker exec → контейнер umeshd ДОЛЖЕН быть запущен.
# (В отличие от genesis plan / init, которые работают через docker run --rm.)

# Точечная правка параметров генезиса
umeshctl genesis set-param --path app_state.staking.params.max_validators --value 100
umeshctl genesis set-time --time 2026-09-01T00:00:00Z   # время старта сети

# Финальный контроль
umeshctl genesis validate      # прогоняет validate-genesis через сам umeshd
umeshctl genesis inspect       # chain_id, denom, число аккаунтов — быстрый взгляд глазами
```

Если генезис уже готов и опубликован координатором, а вы просто одна из
сторон — скачайте его, не создавая заново:

```bash
umeshctl genesis fetch --url https://example.com/genesis.json --sha256 <hash>
```


## 7. Сценарий Validator: подключение валидатора к живой сети

**Ситуация.** Сеть уже запущена: есть опубликованный `genesis.json` и рабочие
пиры. Вы поднимаете новую ноду-валидатор, чтобы догнать цепочку и начать
подписывать блоки. Всё делается с хоста — заходить внутрь контейнера и
редактировать файлы не нужно.

### Что подготовить заранее (чек-лист)

1. **Docker-образ** `umesh-node:latest`. Он собирается из репозитория
   `Node_Umesh`:
   `docker build -t umesh-node:latest -f Dockerfile .`. Без образа `setup
   init` упадёт с `Preflight: image not found`.
2. **YAML-конфиг** по образцу `examples/node-config/validator.yaml` + пароль
   keyring (через один из `--keyring-password-*` флагов или `--auto-password`).
3. **Источник генезиса** — хотя бы один из `join.genesisUrl`, `join.sentryRpc`,
   `join.validatorRpc` (адрес, откуда будет скачан `genesis.json`).
4. **Публичный IP** для продакшена — `network.externalAddress: "<IP>:26656"`.
   Без него нода анонсирует внутренний Docker-bridge адрес (`172.x`), и пиры с
   других хостов до неё не достучатся. Также заранее откройте на файрволе
   `26656/tcp` (p2p) и `26657/tcp` (RPC — нужен sentry-ноде для подключения).

### Порядок, в котором действительно проверяется источник генезиса

Это важная деталь: у каждой роли **свой** порядок опроса `join.*`-источников
(это проверено по коду, не только по тексту README):

| Роль | Порядок опроса источников |
| ---- | --------------------------- |
| `validator` | `sentryRpc/genesis` → `validatorRpc/genesis` → `genesisUrl` |
| `sentry` | `sentryRpc/genesis` → `validatorRpc/genesis` → `genesisUrl` |
| `rpc` | `sentryRpc/genesis` → `genesisUrl` → `validatorRpc/genesis` |

Утилита пробует источники по очереди и останавливается на первом успешном
ответе; если ни один не сработал, в ошибке будет показан результат **каждой**
попытки (`tried [url: err, url: err, ...]`).

### Пошаговый алгоритм

| # | Что делаем | Зачем | Команда |
|---|------------|-------|---------|
| **1** | Готовим конфиг и проверяем источники сети | Скопируйте шаблон и заполните `node.moniker`, `join.*` (≥1 URL), `network.persistentPeers` и **`network.externalAddress: "<ваш_публичный_IP>:26656"`** — в шаблоне стоит заглушка `203.0.113.10`, замените на реальный IP, иначе пиры не подключатся. `chain.chainId`/`chain.denom` при заполненном `join` можно не указывать — подставятся из генезиса. Перед `init` убедитесь, что источник отвечает: любой из `curl` ниже должен вернуть `chain_id`. | `cp examples/node-config/validator.yaml config-validator.yaml && nano config-validator.yaml`<br>`curl -sf http://10.0.0.5:26657/genesis \| jq -r .result.genesis.chain_id`<br>`curl -sf https://example.com/genesis.json \| jq .chain_id` |
| **2** | Инициализируем ноду (офлайн, контейнер остановлен) | Создаёт `./data-validator`, скачивает `genesis.json` по порядку из таблицы выше, извлекает `chain-id`/`bond_denom`, запускает `umeshd init --chain-id`, перезаписывает конфиг скачанным генезисом, применяет production-тюнинг, настраивает `seeds`/`persistentPeers`/`externalAddress`, создаёт кошелёк `validator` в keyring и автоматически бэкапит consensus-ключи. Идемпотентна: повтор без `--force` — предупреждение `already initialized`, ничего не пересоздаётся; `--force` при запущенном контейнере — отказ (сначала `docker compose down`). | `umeshctl init validator --config config-validator.yaml --keyring-password-file ~/.umesh/keyring-pass` |
| **3** | Бэкапим ключи и узнаём адрес кошелька | Автобэкап уже лежит в `./backups-validator/validator-consensus-<ts>/` — скопируйте его на отдельный офлайн-носитель. **Сразу** получите адрес `umesh1...` (офлайн-операция) и запросите перевод на него — пока идёт синхронизация, деньги успеют дойти, и вы сэкономите часы ожидания. | `umeshctl validator backup-consensus --data-dir ./data-validator --output-dir ./my-backup`<br>`umeshctl node keys show validator  # → umesh1...`<br>при `--auto-password` пароль лежит в `~/.config/umesh/keyring.pass` |
| **4** | Запускаем контейнер и открываем порты | Поднимает ноду (образ `umesh-node:latest`, read-only ФС + tmpfs). Файл `docker-compose.yml` лежит в репозитории `Node_Umesh` — запускайте команду из его корня (или укажите `-f`/`--env-file` явно). Сразу проверьте порты: правильный `externalAddress`, но закрытый файрвол → пиры не подключатся → валидатор рискует уйти в jail. | `cd /path/to/Node_Umesh && docker compose --env-file .env.validator --profile validator up -d`<br>`ss -tulpn \| grep 26656; sudo ufw allow 26656/tcp; sudo ufw allow 26657/tcp`<br>`nc -zv <ваш_IP> 26656  # проверка с другой машины`<br>`curl -sf http://<ваш_IP>:26657/net_info \| jq` |
| **5** | Ждём конца синхронизации | Команда регистрации валидатора откажет, пока нода не догнала цепь (`catching_up:false && height>0`). Параллельно должен дойти перевод на кошелёк из шага 3. | `umeshctl node health --wait-sync --timeout 15m` |
| **6** | Проверяем баланс (можно ещё до конца синка) | Для регистрации нужен `amount + расчётная комиссия`. Комиссия по умолчанию считается как `gas-price × gas-limit = 0.0025 × 300000 = 750` (в базовой деноминации). Например, для `amount=5000000uumesh` баланс должен быть **не меньше** `5000750uumesh`. Проверка баланса заранее экономит часы — вы узнаёте о нехватке средств, не дожидаясь окончания синхронизации. | `umeshctl validator check-balance --from <umesh1> --amount 5000000uumesh`<br>`# альтернатива: docker exec umesh-validator umeshd query bank balances <umesh1> --output json` |
| **7** | Регистрируем валидатора (онлайн, контейнер запущен) | Отправляет транзакцию `create-validator` через `docker exec -i`. Публичный ключ (`pubkey`) утилита берёт автоматически из `comet show-validator`, газ считается как `--gas auto --gas-adjustment 1.5` с ценой `--gas-prices 0.0025<denom>`. Перед фактической отправкой автоматически прогоняются три проверки (см. ниже) — при провале любой из них команда откажет ещё до похода в сеть. | `umeshctl validator create --key-name validator --from <umesh1_из_шага3> --moniker "My Node" --amount 5000000uumesh --chain-id <тот_же_chainId> --keyring-pass $(cat ~/.config/umesh/keyring.pass)` |
| **8** | Проверяем регистрацию | `VotingPower` должен стать больше нуля. В самой сети: `staking` должен показывать вашего валидатора, а `signing-info` — не быть `tombstoned`. После этого нода начинает подписывать блоки без перезапуска. | `umeshctl node status validator`<br>`docker exec umesh-validator umeshd query staking validators --home /home/umesh/.umeshd --output json \| grep <ваш_valoper>` |

### Откуда взять деньги на шаг 6

Запросите перевод сразу на шаге 3, пока идёт синхронизация — тогда к моменту
регистрации баланс уже будет на месте.

| Ситуация | Кто создаёт баланс | Куда он попадает | Как пополнить именно вашу новую ноду |
|------|-------------|---------------|--------------------------|
| Генезис (блок 0) | Координатор сети через `genesis.alloc` в `genesis-plan.yaml` | На генезис-адрес оператора (`umeshctl node keys show validator`) | Только если вы сами были генезис-валидатором |
| Живая сеть | Перевод с уже существующего кошелька (вашего или чужого) | На ваш новый `umesh1...` (получен на шаге 3) | Запросите перевод сразу после шага 3 и проверяйте `check-balance`, пока идёт синк |

### Три preflight-проверки перед `validator create`

Эти проверки выполняются автоматически внутри команды `create`; шаг 6
(`check-balance`) фактически прогоняет третью проверку заранее, чтобы не
ждать до самого конца.

1. **Контейнер запущен** — иначе `container "..." is not running — start node
   first: docker compose --env-file .env.validator --profile validator up -d`.
2. **Нода догнала цепь** (`catching_up == false && height > 0`) — иначе `node
   is not fully synced … wait: umeshctl node health --wait-sync`.
3. **Баланса хватает** (`balance ≥ amount + расчётная комиссия`) — иначе
   `insufficient balance: has X, need Y (delegation + fee)`.

### Как это устроено под капотом (полезно при отладке)

- `create-validator` в Cosmos SDK 0.54 требует передавать данные транзакции
  **JSON-файлом**, а не набором CLI-флагов, как было в старых версиях SDK.
  `umeshctl` сам формирует этот JSON: берёт сырой `pubkey` из `comet show-validator` (вида `{"@type":"/cosmos.crypto.ed25519.PubKey","key":"..."}`)
  и подставляет его в поле `pubkey`. Файл временно пишется в
  `./data-validator/config/validator.json` (тот же bind-mount, что смонтирован
  в контейнер) и удаляется сразу после отправки транзакции.
- Пароль keyring передаётся в контейнер через `stdin` (`docker exec -i`) с
  завершающим переводом строки — на экране и в истории команд он не
  светится.
- Опциональные флаги `create`, если нужно переопределить дефолты:
  `--pubkey <json>` (по умолчанию берётся из `comet show-validator`),
  `--gas-prices 0.0025uumesh`, `--gas-adjustment 1.5` (используется только при
  `--gas auto`), `--gas-limit 300000` (фиксированный лимит вместо `auto`),
  `--denom uumesh` (должен совпадать с `bond_denom` сети).
- **Автобэкап на шаге 3** кладётся в `./backups-validator/validator-consensus-<timestamp>/`
  и содержит `priv_validator_key.json` (consensus-ключ), `node_key.json`
  (P2P-идентичность), `priv_validator_state.json` (высота последней подписи).
  Права — `0600` на файлы, `0700` на каталог.
- **Критично важно:** никогда не восстанавливайте `priv_validator_state.json`
  на уже работающем валидаторе и никогда не делайте `--force` при запущенном
  контейнере — риск **double-sign**, что ведёт к перманентному jail/tombstone
  валидатора (это необратимо на уровне протокола). Также возможна блокировка
  БД (`DB lock`).

### Диагностика сети до/после запуска (предупреждает, но не блокирует)

- Доступность источников: `curl -sf <genesisUrl>`, `curl -sf <sentryRpc>/genesis`;
  итоговая ошибка `init <role>` покажет попытки по каждому источнику.
  `umeshctl genesis fetch --dry-run` — то же самое, но без записи на диск.
- Совпадение `chainId`/`denom`: `umeshctl genesis fetch --url <join> --output /tmp/g.json && umeshctl genesis inspect --file /tmp/g.json` — сверьте с YAML до запуска `init <role>`.
- P2P-доступность: `umeshctl ops doctor --check p2p` и `umeshctl ops verify --cross-role` — проверяют `externalAddress` на предмет `172.x`, доступность порта (`nc -zv`) и число пиров (`/net_info`).

### Справочник команд валидатора

| Команда | Когда нужна | Что делает |
|---------|-------------|------------|
| `node keys list` | Инициализация | Список ключей (офлайн) |
| `node keys show validator` | Шаг 3 | Адрес кошелька `umesh1...` (офлайн) |
| `validator operator-address --key-name validator` | Регистрация | Адрес `umeshvaloper1...` (онлайн) |
| `validator create ...` | Шаг 7 | Зарегистрировать валидатора в живой сети |
| `validator generate-gentx --key-name validator --chain-id <id> --output ./gentx` | Mainnet Ritual (**до** запуска сети) | Сгенерировать gentx для включения в генезис |
| `init validator --config <yaml>` | Шаг 2 | Инициализация ноды (`chainId`/`denom` авто-извлекаются из генезиса) |
| `validator check-balance --from <umesh1> --amount <amt>` | Шаг 6 | Проверка баланса без ожидания синхронизации |
| `validator backup-consensus [--output-dir <dir>]` | Шаг 3 | Бэкап консенсус-ключей |
| `validator signing-info --cons-addr <valcons...>` | После даунтайма | Пропущенные блоки, статус tombstone |
| `validator unjail --key-name validator --chain-id <id> --keyring-pass <пароль>` | После даунтайма | Разжаловать валидатора |
| `node keys export <name>` / `import <name>` | Перенос ноды | Экспорт/импорт ключей (офлайн) |
| `node keys delete <name>` | — | Удалить ключ (**необратимо!**) |

> Офлайн-операции `node prune`, `node snapshot create/list/restore`,
> `ops restore`, `init <role> --force` требуют **остановленного** контейнера —
> при запущенном покажут предупреждение `stop node first to avoid DB
> lock/corruption`.


## 8. Сценарий Sentry: публичный щит валидатора

**Что это и зачем.** Sentry — публичный узел, стоящий в интернете и
принимающий все входящие подключения из сети, чтобы скрыть валидатора от
прямых атак (DDoS, попытки эксплуатации). Sentry **не подписывает блоки** — у
него нет `priv_validator_key.json` и кошелька, поэтому пароль keyring вообще
не нужен. Типичная связка: валидатор подключается только к sentry (у
валидатора `p2p.pex=false`, он никого не ищет сам), а sentry общается со всей
остальной сетью (`p2p.pex=true`).

### Какие данные понадобятся и откуда их взять

| Данные | Что это | Откуда взять |
| ------ | ------- | ------------ |
| `node.moniker` | Имя ноды | Придумайте сами, например `my-sentry-1` |
| `join.*` | Источник `genesis.json`: хотя бы один из `genesisUrl`, `sentryRpc`, `validatorRpc` | У координатора сети; `validatorRpc` — это адрес вашей же валидатор-ноды: `http://<IP_валидатора>:26657` |
| `chain.chainId` / `chain.denom` | ID сети и деноминация | Указывать не обязательно — возьмутся из генезиса автоматически |
| `network.externalAddress` | Ваш **публичный IP**:`26656`, например `203.0.113.20:26656` | У провайдера VPS. Пустым не оставляйте — иначе нода анонсирует внутренний `172.x`, и пиры не смогут подключиться |
| **NodeID валидатора** | Идентификатор валидатора для `persistentPeers` | На машине валидатора, **без запуска RPC**: `cat ./data-validator/config/node_key.json` либо `docker run --rm -v ./data-validator:/home/umesh/.umeshd umesh-node:latest umeshd comet show-node-id --home /home/umesh/.umeshd` |
| `network.persistentPeers` | Пиры сети: **обязательно** укажите валидатора как `<NodeID_валидатора>@<приватный_IP_валидатора>:26656`, плюс остальные пиры | NodeID — из строки выше, IP валидатора — из вашей приватной сети |
| `network.usePrivate: true` | Скрыть IP валидатора от остальной сети (unconditional/private peer) | Включайте, если валидатор не должен «светиться» наружу; для этого нужен доступный `validatorRpc` в момент инициализации |

> Если `validatorRpc` уже доступен на момент `init sentry`, sentry попробует
> сам получить NodeID валидатора по `GET /status`. Это не обязательное
> условие: если RPC недоступен, `init sentry` выдаст предупреждение и
> продолжит работу — тогда впишите `persistentPeers` вручную по таблице выше.

### Какие порты открыть

По умолчанию тюнинг-профиль sentry включает не только P2P, но и публичные
REST/gRPC-эндпоинты (тот же профиль, что и у роли `rpc`) — так что если вы не
хотите обслуживать внешних клиентов через sentry, закройте 1317/9090 на
файрволе, оставив только 26656/26657.

| Порт | Что слушает | Кому нужен |
| ---- | ----------- | ---------- |
| `26656` | P2P | Всем пирам сети (обязательно) |
| `26657` | Tendermint RPC | Публичный доступ (кошельки, скрипты, RPC-ноды) |
| `1317` | REST API | Публичные API (включён по умолчанию тюнинг-профилем) |
| `9090` | gRPC + gRPC-Web (на том же порту; отдельный `9091` нужен только если вы сами ставите перед gRPC Envoy) | Серверные и браузерные dApps |

Метрики (`26660`) и data companion (`26658`) по умолчанию слушают только
`localhost` и наружу не публикуются.

### Что подготовить заранее

1. **Образ** `umesh-node:latest` — собирается в отдельном репозитории
   `Node_Umesh`: `docker build -t umesh-node:latest -f Dockerfile .`.
2. **YAML-конфиг** — скопируйте шаблон `examples/node-config/sentry.yaml` и
   заполните данными из таблицы выше: `node.moniker`, `join.*`,
   `network.externalAddress`, `network.persistentPeers`, при необходимости
   `network.usePrivate`.

### Пошаговый алгоритм

| # | Что делаем | Зачем | Команда |
|---|------------|-------|---------|
| **1** | Готовим конфиг и проверяем сеть | Заполните шаблон данными из таблицы выше. Убедитесь, что источник генезиса отвечает — иначе `init` упадёт. | `cp examples/node-config/sentry.yaml config-sentry.yaml && nano config-sentry.yaml`<br>`umeshctl genesis fetch --url http://<validatorRpc>/genesis --dry-run` — покажет `chainId`, ничего не записывая на диск |
| **2** | Инициализируем ноду (офлайн, контейнер остановлен) | Создаёт `./data-sentry`, скачивает `genesis.json` (порядок: `sentryRpc → validatorRpc → genesisUrl`), применяет production-тюнинг (`p2p.pex=true`, лимиты пиров 40 входящих / 20 исходящих — руками ничего настраивать не нужно), прописывает `seeds`/`persistentPeers`/`externalAddress`; при `usePrivate=true` добавляет валидатора в `private_peer_ids`. | `umeshctl init sentry --config config-sentry.yaml --data-dir ./data-sentry`<br>пересоздать без смены NodeID: `--force --keep-keys` |
| **3** | Сразу бэкапим NodeID | `node_key.json` — это **личность sentry**, его NodeID. Без бэкапа после `--force` (без `--keep-keys`) ID пересоздастся, и валидатор потеряет связь со своим щитом. | `cp ./data-sentry/config/node_key.json ./backups-sentry/node_key.$(date +%s).json && chmod 600 ./backups-sentry/node_key.*`<br>при `--auto-password` пароль лежит в `~/.config/umesh/keyring.pass` |
| **4** | Быстрый старт (только для большой живой сети, >1M блоков) | Молодая сеть — этот шаг можно пропустить. Старая сеть — синхронизация с нуля займёт дни. Выберите **A) StateSync** или **B) Snapshot**. `trust_hash` вы получаете сами — с доверенной RPC-ноды. | A) `umeshctl node config set statesync.rpc_servers "http://node1:26657,http://node2:26657" --data-dir ./data-sentry`<br>`umeshctl node config set statesync.trust_height 12345 --data-dir ./data-sentry`<br>`umeshctl node config set statesync.trust_hash <hash> --data-dir ./data-sentry`<br>B) `umeshctl node snapshot restore --from ./snapshots --data-dir ./data-sentry` (контейнер должен быть остановлен) |
| **5** | Привязываем валидатор к sentry (щит) | На машине **валидатора** добавьте sentry в `persistent_peers`, чтобы валидатор соединялся только через щит, а не напрямую с сетью. | На валидаторе: `umeshctl node peers add <sentryNodeID>@<sentryIP>:26656 --data-dir ./data-validator`<br>или правкой `data-validator/config/config.toml`: `persistent_peers = "<sentryNodeID>@<sentryIP>:26656"` |
| **6** | Запускаем sentry | Поднимите контейнер из каталога `Node_Umesh` (там лежат `docker-compose.yml` и `.env.sentry`). | `cd Node_Umesh && docker compose --env-file .env.sentry --profile sentry up -d`<br>`docker logs -f umesh-sentry` |
| **7** | Ждём синхронизации и проверяем связку | Sentry должен догнать цепь (`catching_up=false`) и иметь пиров; валидатор должен видеть sentry в своих подключениях. | `umeshctl node health --data-dir ./data-sentry --wait-sync --timeout 15m`<br>`curl -sf http://<sentryIP>:26657/net_info \| jq .result.n_peers` — должно быть >0<br>`grep -E 'external_address\|private_peer_ids' ./data-sentry/config/config.toml` — `external_address` не должен быть `172.x`, а `private_peer_ids` должен содержать NodeID валидатора (если включён `usePrivate`)<br>`umeshctl ops doctor --check p2p --data-dir ./data-sentry` и `umeshctl ops verify --cross-role` — предупреждения здесь не фатальны |

### Частые вопросы

- **Нужен ли пароль keyring?** Нет: sentry не подписывает блоки и не создаёт
  кошельков.
- **Нужно ли тюнить `config.toml` руками (pex, лимиты, laddr)?** Нет.
  `umeshctl` сам ставит `p2p.pex=true`, `max_inbound=40`, `max_outbound=20`,
  `p2p.laddr=tcp://0.0.0.0:26656`, `external_address` — из вашего конфига.
  Проверить: `grep p2p.pex ./data-sentry/config/config.toml` → должно быть
  `true`.
- **Где взять NodeID валидатора?** На машине валидатора, без запуска RPC:
  `cat ./data-validator/config/node_key.json` или `umeshd comet show-node-id
  --home /home/umesh/.umeshd`.
- **Что будет при `--force`?** `node_key.json` пересоздастся — NodeID
  изменится. Чтобы сохранить его, используйте `--force --keep-keys` либо
  ручной бэкап из шага 3.
- **`usePrivate` не сработал?** Это нормально, если на момент инициализации
  `validatorRpc` был недоступен. Добавьте валидатора в `persistentPeers`
  вручную, в виде `<NodeID>@IP:26656`.
- **Порт закрыт?** `umeshctl` не управляет файрволом. Откройте `26656/tcp`
  (p2p) на VPS/у провайдера вручную и проверьте доступность с другой машины:
  `nc -zv <ваш_IP> 26656`.

### Полная связка «с нуля» (если обе ноды ещё не проинициализированы)

```bash
# 1) На валидаторе узнаём свой NodeID (офлайн, без запуска RPC)
VAL_ID=$(docker run --rm -v ./data-validator:/home/umesh/.umeshd umesh-node:latest umeshd comet show-node-id --home /home/umesh/.umeshd)
echo "Validator NodeID: $VAL_ID"
# Вставьте значение в config-sentry.yaml: persistentPeers: "${VAL_ID}@10.0.0.10:26656"

# 2) На sentry вписываем его NodeID в persistentPeers (шаг 1) и запускаем init (шаг 2)

# 3) На валидаторе добавляем sentry как единственный persistentPeer
SENTRY_ID=$(docker run --rm -v ./data-sentry:/home/umesh/.umeshd umesh-node:latest umeshd comet show-node-id --home /home/umesh/.umeshd)
umeshctl node peers add ${SENTRY_ID}@<sentryIP>:26656 --data-dir ./data-validator
```


## 9. Сценарий RPC: публичная точка входа

**Что это и зачем.** RPC-нода — публичная точка входа в сеть для кошельков,
эксплореров и скриптов: они ходят к ней за данными и отправляют транзакции.
Нода сама синхронизирует цепь и отдаёт состояние через стандартные API. В
отличие от валидатора, она **не подписывает блоки** — у неё нет
`priv_validator_key.json` и кошелька, поэтому пароль keyring не нужен. В
отличие от sentry, она не «прячет» валидатора, а обслуживает всех клиентов
напрямую.

### Какие данные понадобятся и откуда их взять

| Данные | Что это | Откуда взять |
| ------ | ------- | ------------ |
| `node.moniker` | Имя ноды | Придумайте сами, например `my-rpc` |
| `node.pruning` | Локальная стратегия хранения (`app.toml`): `custom` (дефолт для RPC, окно 1000 блоков / интервал очистки 100), `everything` (только broadcast, без истории), `nothing` (archive, хранит всё), `default` | Задаётся в YAML полем `node.pruning`. Обратите внимание: поля `chain.pruning` **не существует** — pruning это свойство конкретного инстанса ноды, а не свойство сети |
| `join.sentryRpc` / `join.genesisUrl` | Источник `genesis.json`: хотя бы один из `sentryRpc`, `genesisUrl` | `sentryRpc` — ваша собственная sentry-нода (`http://<IP_sentry>:26657`) либо публичный RPC координатора; `genesisUrl` — прямая ссылка на файл (например, S3/CDN, `https://example.com/genesis.json`) |
| `chain.chainId` / `chain.denom` | ID сети и деноминация | Указывать не обязательно — возьмутся из генезиса автоматически |
| `network.persistentPeers` / `seeds` | Пиры для P2P, вида `NodeID@IP:26656`; для мгновенной доставки транзакций (`broadcast_tx_sync`) добавьте свою sentry как `persistentPeer` | У координатора, из `addrbook` sentry-ноды или через `node peers list`; NodeID sentry — `comet show-node-id --home ./data-sentry` |
| `network.externalAddress` | Ваш публичный `IP:26656` | У провайдера VPS; нужен только если хотите, чтобы к вам подключались входящие пиры |

### Какие порты слушает RPC

| Порт | Что слушает | Кому нужен |
| ---- | ----------- | ---------- |
| `26657` | Tendermint RPC | Кошельки, скрипты, сам `umeshctl` |
| `1317` | REST API | Эксплореры, веб-приложения |
| `9090` | gRPC + gRPC-Web (на том же порту; отдельный `9091` нужен только если вы сами ставите перед gRPC Envoy) | Серверные и браузерные dApps |

Метрики (`26660`) и data companion (`26658`) по умолчанию слушают только
`localhost` и наружу не публикуются. Индексатор транзакций (`tx_index.indexer`) для RPC уже включён (`"kv"`) — у валидатора он, наоборот, выключен (`"null"`), это часть
дефолтного профиля тюнинга.

### Что подготовить заранее

1. **Образ** `umesh-node:latest` — собирается в отдельном репозитории
   `Node_Umesh`: `docker build -t umesh-node:latest -f Dockerfile .`.
2. **YAML** — `cp examples/node-config/rpc.yaml config-rpc.yaml`, заполните:
   `node.moniker` и `node.pruning` (`custom` для баланса истории/размера диска,
   `everything` для лёгкой ноды без истории), `join.sentryRpc` **или**
   `genesisUrl`, `network.persistentPeers` с вашей sentry для быстрой доставки
   транзакций, `network.externalAddress` — если хотите принимать P2P.

### Пошаговый алгоритм (офлайн init → старт)

| # | Что делаем | Зачем | Команда |
|---|------------|-------|---------|
| **1** | Готовим конфиг и проверяем источник | Заполните шаблон. Убедитесь, что генезис доступен — при провале всех источников `init <role>` покажет результат каждой попытки. | `cp examples/node-config/rpc.yaml config-rpc.yaml && nano config-rpc.yaml`<br>`umeshctl genesis fetch --url http://<sentryRpc>/genesis --dry-run` (или `--url https://example.com/genesis.json --dry-run`) |
| **2** | Инициализируем ноду (офлайн, контейнер остановлен) | Создаёт `./data-rpc`, скачивает `genesis.json`, включает `rpc.laddr 0.0.0.0:26657` с открытым CORS, применяет тюнинг-профиль RPC (`p2p.pex true`, лимиты пиров 60 входящих / 40 исходящих, `pruning custom 1000/100` — или другое значение из `node.pruning`, `api 1317`, `grpc 9090 + grpc-web`, `tx_index kv`), прописывает `seeds`/`persistentPeers`/`externalAddress`. Идемпотентна. | `umeshctl init rpc --config config-rpc.yaml --data-dir ./data-rpc`<br>Пересоздать без смены NodeID: `--force --keep-keys` |
| **3** | Сразу бэкапим NodeID | `node_key.json` — личность RPC-ноды. | `cp ./data-rpc/config/node_key.json ./backups-rpc/node_key.$(date +%s).json && chmod 600 ./backups-rpc/node_key.*`<br>при `--auto-password` пароль лежит в `~/.config/umesh/keyring.pass` |
| **4** | Запускаем контейнер | Профиль `rpc` в репозитории `Node_Umesh`. | `cd Node_Umesh && docker compose --env-file .env.rpc --profile rpc up -d`<br>`docker logs -f umesh-rpc` |
| **5** | Открываем порты и ждём синхронизации | RPC должна догнать цепь (`catching_up=false`). При `pruning="custom" 1000/100` глубина хранимой истории — примерно 1000 блоков (~1.5 часа при блоке в 5 секунд); `interval` — это лишь частота фоновой очистки, а не размер окна. | `ss -tulpn \| grep -E '26657\|1317\|9090'`<br>`umeshctl node health --data-dir ./data-rpc --wait-sync --timeout 15m`<br>`curl -sf http://<IP>:26657/status \| jq .result.sync_info.catching_up`  — ожидаем `false`<br>`curl -sf http://<IP>:1317/cosmos/base/tendermint/v1beta1/node_info \| jq` |
| **6** | Проверяем публичные API и связку с sentry | `broadcast_tx_sync` должен мгновенно доходить до sentry через `persistentPeers`. | `curl -sf http://<IP>:26657/net_info \| jq .result.n_peers`  (должно быть >0)<br>`grep -E 'pruning\|tx_index\|external_address' ./data-rpc/config/{app,config}.toml`  — `pruning="custom"` `1000/100`, `tx_index="kv"`<br>`umeshctl ops doctor --check p2p --data-dir ./data-rpc`  (предупреждение не фатально) |
| **7** | Быстрый старт для старой сети (опционально, >1M блоков) | Синхронизация с нуля — дни. Выберите StateSync (`trust_height`/`trust_hash` с доверенной RPC) или Snapshot (`node snapshot restore`). | Как в §8 шаг 4: `node config set statesync.*` либо `node snapshot restore --from ./snapshots --data-dir ./data-rpc` (нода остановлена; sentry-нода уже настроена как источник со `state-sync.snapshot-interval=5000`) |

### Частые вопросы

- **Нужно ли указывать `chain.pruning`?** Нет, такого поля не существует —
  pruning задаётся **локально**, полем `node.pruning`, потому что это
  свойство конкретного инстанса ноды, а не свойства сети. На одной и той же
  цепи у валидатора может стоять `custom 10000/1000`, у RPC — `custom 1000/100`,
  у archive-ноды — `nothing`.
- **Нужен ли пароль keyring?** Нет: RPC не создаёт кошельков.
- **Где взять `join.sentryRpc`, если своей sentry нет?** Подойдёт любой
  публичный RPC координатора сети; `genesisUrl` (S3/CDN) — фоллбек, когда sentry недоступна.
- **Что будет при `--force`?** `node_key.json` пересоздастся — NodeID
  изменится. Для RPC это не критично для консенсуса, но лучше сохранить через
  `--force --keep-keys`.
- **Исторические запросы (например, «баланс на высоте N-1000») не работают?**
  При `pruning="custom" 1000/100` окно хранения — около 1000 блоков вглубь.
  Для более глубоких запросов нужен `pruning="nothing"` (archive-режим). При
  `everything` доступны только самые свежие данные (чистый broadcast). `default`
  — это значение по умолчанию из SDK.
- **Как настроить upstream для RPC-ноды?**
  `umeshctl rpc set-upstream --rpc-upstream http://sentry:26657 --rest-upstream http://sentry:1317 --p2p-upstream <nodeID>@<IP>:26656`
- **Как настроить CORS для публичного RPC?**
  `umeshctl rpc configure-cors --origins https://app.example.com,https://app2.example.com`
  или `umeshctl rpc configure-cors --disable` для отключения CORS.


## 10. Эксплуатация после запуска

Эти команды одинаково применимы к любой роли — просто указывайте нужный
`--data-dir`/`--container`, если работаете не с ролью по умолчанию
(`validator`).

### Проверка здоровья

Первое, что нужно сделать после старта контейнера — убедиться, что нода
жива, синхронизируется и видит пиров.

```bash
umeshctl node status sync        # высота, catching up, voting power
umeshctl node info               # moniker, network, версия бинарника (alias: status node)
umeshctl node status peers       # сколько пиров подключено
umeshctl node status validator   # данные о вашем валидаторе
umeshctl node status docker      # healthcheck Docker-контейнера
umeshctl node health             # быстрая проверка «жива ли нода»
umeshctl node health --wait-sync --timeout 5m   # подождать до конца синхронизации
umeshctl node peers list         # seeds + persistent peers из текущего конфига
```

### Конфигурация и производительность

Вместо ручного редактирования TOML-файлов:

```bash
umeshctl node config get consensus.timeout_commit            # прочитать значение
umeshctl node config set p2p.max_num_inbound_peers 60        # изменить значение
umeshctl node config set rpc.laddr "tcp://0.0.0.0:26657"
umeshctl node config diff --output json                                    # сравнить с рекомендациями (роль авто-определяется)
umeshctl setup tune                                          # применить рекомендации целиком (роль авто-определяется)
umeshctl init validator --dry-run                            # dry-run для init
umeshctl ops restore --dry-run --from ./backups --role validator
umeshctl node prune --dry-run
```

`setup tune` полезна, когда роль ноды сменилась или конфигурация
настраивалась давно вручную и её нужно привести к рекомендациям — без полной
переинициализации ноды. Роль определяется автоматически из `.node-info`; при необходимости можно переопределить флагом `--role`.

Управление списком пиров:

```bash
umeshctl node peers add <node-id>@<ip>:26656     # добавить persistent peer
umeshctl node peers add --unconditional <node-id> # пир, которого нельзя вытеснить лимитом
umeshctl node peers remove <node-id>@<host>:<port>  # удалить пир (<node-id> тоже принимается)
umeshctl node peers clear                        # очистить persistent_peers полностью
# все команды add/remove/clear поддерживают -y/--yes для CI
```

### Синхронизация новой ноды: statesync и snapshot

Новая нода может очень долго синхронизировать всю историю с нуля. State
sync — способ ускорить это: нода скачивает готовый снапшот состояния и
проверяет его по доверенным высоте и хэшу, которые вы задаёте сами (получая
их с доверенной RPC-ноды).

```bash
umeshctl node statesync enable --trust-height 12345 --trust-hash <hash> \
  --rpc-servers http://node1:26657,http://node2:26657
umeshctl node statesync disable
umeshctl node statesync show                      # показать текущие настройки
```

Снапшоты для state sync создаются и хранятся прямо на узле — их можно
создавать, просматривать и восстанавливать:

```bash
umeshctl node snapshot create
umeshctl node snapshot list
umeshctl node snapshot list -o json               # структурированный вывод: height/format/chunks
umeshctl node snapshot restore --from ./snapshots
```

### Логи, обрезка данных, апгрейды

```bash
umeshctl node logs                          # все логи
umeshctl node logs --level error --since 1h # только ошибки за последний час
umeshctl node logs --module consensus       # логи модуля consensus
umeshctl node logs -f                       # следовать за логами в реальном времени

umeshctl node prune --keep-recent 1000      # освободить место, оставив N последних блоков

umeshctl node upgrade info                  # текущая версия бинарника
# umeshctl node upgrade prepare --version v0.2.0
# ^ команда объявлена в CLI, но пока НЕ реализована: автоматическое скачивание
#   бинарника и проверка контрольной суммы ещё не подключены. Явно вернёт
#   ошибку "upgrade prepare is not implemented yet".
```


## 11. Резервное копирование и восстановление

**`umeshctl ops backup`** — важный шаг обслуживания, но важно понимать, что
именно он копирует, а что — нет.

```bash
umeshctl ops backup --output ./backups
```

**Что реально попадает в бэкап** (проверено по коду):

| Роль | Файлы в бэкапе |
| ---- | -------------- |
| `sentry` | `node_key.json`, `genesis.json` |
| `validator` / `genesis` / `rpc` | `priv_validator_key.json`, `node_key.json`, `genesis.json`, `priv_validator_state.json` |

> ⚠️ **Важно: `ops backup` НЕ копирует каталог `keyring/`.** Несмотря на то,
> что кошелёк — единственное, чем распоряжаются деньгами ноды, текущая
> реализация `ops backup` его не бэкапит. При этом `ops restore` пытается
> восстановить keyring из подкаталога `keyring-file` внутри бэкапа — но так
> как `ops backup` туда ничего не кладёт, восстанавливаться будет нечем (это
> не приводит к ошибке, просто восстановление keyring молча пропускается).
> **Практический вывод:** бэкапьте `keyring/` отдельно и вручную, например:
> ```bash
> cp -r ./data-validator/keyring ./backups/keyring-$(date +%s)
> chmod -R 600 ./backups/keyring-*
> ```
> либо используйте `umeshctl node keys export <name>` для каждого ключа
> по отдельности (тоже требует своего пароля на экспорт).
>
> **Важно:** файл пароля keyring (при `--auto-password`) теперь хранится в
> XDG config dir: `~/.config/umesh/keyring.pass` — бэкапьте его **отдельно**
> от данных ноды. Команда `ops backup` выводит предупреждение с путём.

**Ещё одна деталь, важная для планирования:** `ops backup` требует **запущенного**
контейнера (файлы читаются через `docker exec cat ...`), а `ops restore`,
наоборот, требует **остановленного** контейнера (пишет файлы напрямую на
хостовый диск, что при работающей ноде рискует повредить активные ключи).

```bash
# Восстановление (нода должна быть остановлена!)
umeshctl ops restore --from ./backups/20260810 --role validator
umeshctl ops restore --from ./backups/20260810 --role sentry
umeshctl ops restore --from ./backups/20260810 --role validator --dry-run  # preview
# После восстановления автоматически восстанавливается genesis.json (если был в бэкапе)
# и создаётся/обновляется .node-info с ролью, chain_id, node_id и genesis_time.
```

Диагностика, если что-то пошло не так:

```bash
umeshctl ops doctor                                  # архитектура хоста, NTP, .gitignore, готовность
umeshctl ops doctor --all                            # все проверки (включая container-health, wasmvm)
umeshctl ops doctor --check container-health         # отдельная точечная проверка
umeshctl ops doctor --output json                    # машиночитаемый отчёт (exit != 0 при неуспехе)
umeshctl ops verify                                  # проверка роли и связности по RPC (роль авто-определяется)
umeshctl ops verify --cross-role                     # согласованность связки валидатор ↔ sentry
```


## 12. Genesis plan (YAML) — подробный разбор

Декларативный план (пример — `examples/genesis-plan.yaml`) полностью
описывает стартовое состояние сети. Это единственный источник истины для
генезиса — вместо того, чтобы руками редактировать `genesis.json`, вы
описываете желаемое состояние декларативно, а `umeshctl` собирает файл сам.

- **`chain`** — `chain_id`, `moniker`, `denom`, `decimals`, `genesis_time`
  (`now` или пусто = старт сразу же, либо конкретное время в формате
   RFC3339), опционально `denom_uri` и `constitution`, а также
   **consensus-параметры**. Поля consensus, оставленные пустыми, берутся из
   production-дефолтов, совпадающих с Cosmos Hub (`cosmoshub-4`). **Важно:**
   consensus-параметры фиксируются в генезисе навсегда — поменять их после
   запуска сети можно будет только через governance-предложение, а не правкой
файла. `denom_uri` попадает в `bank.denom_metadata[*].uri` и применяется
    в `genesis plan` и `init genesis`.
- **`tokenomics`** — `total_supply` (указывается в базовой деноминации,
  например `1_000_000_000 UMESH = 1000000000000000 uumesh`) и аллокации:
  - `base_account` — обычный баланс без vesting;
  - `delayed_vesting` / `continuous_vesting` — постепенная разблокировка
    (`start_time`, `end_time`, при необходимости `cliff_duration`,
    `vesting_duration`);
  - `validator_set` — токены, застейканные генезис-валидаторами.
- **`validation`** — правила децентрализации при формировании плана: максимум
  на одну аллокацию, максимум суммарно на инсайдеров (foundation/team/
  investors), минимум валидаторов, куда девать остаток при округлении
  (`dust_destination`).
- **`modules`** — параметры модулей `staking`, `distribution`, `bank`, `mint`,
  `gov`, `slashing`, `wasm`, `epochs`, `protocolpool`:
  - `staking`: `max_validators`, `max_entries`, `historical_entries`,
    `unbonding_time`, `bond_denom`, `min_commission_rate`.
  - `distribution`: `community_tax`, `withdraw_addr_enabled`. *Устарели и
    удалены начиная с SDK 0.47+: `base_proposer_reward`,
    `bonus_proposer_reward` — если указаны, цепь их просто проигнорирует.*
  - `bank`: **`default_send_enabled`** (по умолчанию `true`).
  - `mint`: `mint_denom`, `inflation_rate_change`, `inflation_max`,
    `inflation_min`, `goal_bonded`, `blocks_per_year`, `max_supply`.
  - `gov`: `min_deposit`, `expedited_min_deposit`, `max_deposit_period`,
    `voting_period`, `quorum`, `threshold`, `veto_threshold`,
    `min_initial_deposit_ratio`, `burn_vote_quorum`,
    `burn_proposal_deposit_prevote`, **`expedited_voting_period`**,
    **`expedited_threshold`**, **`proposal_cancel_ratio`**,
    **`proposal_cancel_dest`**, **`burn_vote_veto`**, **`min_deposit_ratio`**,
    **`starting_proposal_id`** (всё перечисленное жирным — новое в SDK 0.50+).
  - `slashing`: `signed_blocks_window`, `min_signed_per_window`,
    `downtime_jail_duration`, `slash_fraction_double_sign`,
    `slash_fraction_downtime`.
  - `wasm`: `code_upload_access`, `instantiate_default_permission`.
  - `epochs`: список таймеров с `identifier`, `duration`, опционально
    `start_time`.
  - `protocolpool`: `enabled_distribution_denoms`, `distribution_frequency`,
    `continuous_funds` (список объектов: `recipient`, `percentage`,
    `expiry`). *Примечание:* модуль `protocolpool` удалён в SDK 0.55; параметры
    применяются только если модуль скомпилирован в `umeshd` (иначе — с предупреждением),
    а при апгрейде на 0.55+ становятся мёртвыми.*
- **`soft_launch`** — поэтапный запуск сети: `disable_bank_send` и
  `disable_ibc_transfer` временно отключают переводы, `allow_wasm_instantiate`
  управляет правом инстанцировать контракты (`Everybody`/`Nobody`).
  `allow_staking` и `allow_gov` пока зарезервированы и не применяются
  (стейкинг обязателен для работы консенсуса и не может быть отключён этим
  механизмом). **`disable_inflation`** — если `true`, на период мягкого
  запуска `inflation_min` и `inflation_max` принудительно ставятся в `0`
  (минт не выпускает новые токены).

Работа с планом:

```bash
umeshctl genesis validate-plan --config genesis-plan.yaml   # проверить план
umeshctl genesis report --config genesis-plan.yaml          # отчёт по аллокациям
umeshctl genesis plan --config genesis-plan.yaml --auto-password   # создать генезис (пароль в ~/.config/umesh/keyring.pass)
umeshctl genesis plan --config genesis-plan.yaml --force
umeshctl genesis plan --config genesis-plan.yaml --force --keep-keys
```


## 13. Справочник команд

Подробная справка по любой команде: `umeshctl <команда> --help`.

### `init` — инициализация ноды

| Команда | Зачем |
| ------- | ----- |
| `init <role>` | Инициализировать ноду одной из ролей: `genesis` / `validator` / `sentry` / `rpc`. Обязательно `--config <файл>`, `--force` (контейнер остановлен), `--keep-keys`/`--auto-password`/`--keyring-password-*`, плюс override-флаги `--chain-id --moniker --denom --min-gas-price --environment --pruning --genesis-url --sentry-rpc --validator-rpc --rpc-upstream/--rest-upstream/--p2p-upstream --seeds --persistent-peers --external-address/--external-port --public-ip --use-private --addrbook-url --addrbook-sha256 --genesis-sha256` (см. [справочник флагов](#справочник-флагов-команд)) |

### `validator` — жизненный цикл валидатора (после запуска)

| Команда | Зачем |
| ------- | ----- |
| `validator create [--yes/-y]` | Зарегистрировать валидатора в живой сети (подтверждение; `--yes` для CI) |
| `validator check-balance` | Ранняя проверка баланса (до синхронизации) |
| `validator generate-gentx` | Сгенерировать gentx для Mainnet Ritual |
| `validator operator-address [--output table\|json\|yaml]` | Получить адрес оператора (`umeshvaloper...`) |
| `validator signing-info [--output table\|json\|yaml]` | Информация о подписании (пропуски, tombstone) |
| `validator unjail` | Разджаловать валидатора после даунтайма |
| `validator backup-consensus` | Бэкап консенсус-ключей (`priv_validator_key.json`, `node_key.json`, `priv_validator_state.json`) |

### `sentry` — управление sentry-нодой (после запуска)

| Команда | Зачем |
| ------- | ----- |
| `sentry connect` | Соединить sentry с валидатором (получить NodeID, настроить пиры). После успеха показывает next-step hint: `umeshctl sentry update --peer-id <sentryID>`. |
| `sentry update` | Обновить список пиров валидатора на sentry (поддерживает `-y/--yes`). |

### `rpc` — настройки публичного RPC (после запуска)

| Команда | Зачем |
| ------- | ----- |
| `rpc set-upstream` | Настроить upstream RPC/REST/P2P endpoints. Summary `Updated N of 3` считается только по успешным записям. |
| `rpc configure-cors` | Настроить `rpc.cors_allowed_origins` |

### `genesis` — утилиты генезиса (для координаторов)

| Команда | Зачем |
| ------- | ----- |
| `genesis plan` | Создать production-генезис из YAML-плана (`--auto-password` для автогенерации пароля) |
| `genesis validate-plan` | Проверить план без выполнения |
| `genesis report` | Показать отчёт по аллокациям (`--output json`) |
| `genesis add-account` | Инкрементально добавить аккаунт в генезис (base / delayed_vesting / continuous_vesting / module_account); флаги `--key-name/--address/--mnemonic`, `--type`, `--amount`, `--start-time/--end-time`, `--module-name` (см. [справочник флагов](#справочник-флагов-команд)) |
| `genesis add-validator` | Добавить валидатора в генезис и сгенерировать gentx |
| `genesis fetch --url <url> [--sha256]` | Скачать готовый генезис у координатора |
| `genesis inspect [--output table|json|yaml]` | Показать chain_id, denom, число аккаунтов |
| `genesis validate` | Прогон `validate-genesis` через `umeshd` |
| `genesis set-param --path <путь> --value <значение>` | Изменить параметр в генезисе |
| `genesis set-time --time <RFC3339>` | Задать время старта сети |
| `genesis collect-gentx --repo <url>` | Собрать gentx валидаторов из репозитория в генезис |

### `setup` — подготовка и планирование (до запуска)

| Команда | Зачем |
| ------- | ----- |
| `setup tune [--role]` | Применить production-профиль тюнинга к конфигам (роль авто-определяется) |
| `setup keys add <имя>` | Создать ключ до запуска сети |
| `setup validate --config <файл> [--output table|json|yaml]` | Проверить YAML-конфиг ноды на корректность. `--output json|yaml` для машинного чтения. |

### `node` — эксплуатация (после запуска)

| Команда | Зачем |
| ------- | ----- |
| `node status sync [--output table\|json\|yaml]` | Высота, catching up, voting power (JSON теперь совпадает с table) |
| `node info [--output table|json|yaml]` | Moniker, network, версия (alias: `status node`) |
| `node status peers [--output table\|json\|yaml]` | Число подключённых пиров + configured peer lists. Всегда показывает `Connected Peers: 0` если пиров нет. JSON-ключи: `connected_peers`, `seeds`, `persistent_peers`, `unconditional_peer_ids`, `private_peer_ids` |
| `node status validator [--output table|json|yaml]` | Данные валидатора |
| `node status docker [--output table|json|yaml]` | Healthcheck контейнера |
| `node health [--wait-sync] [--output table\|json\|yaml]` | Быстрая проверка «жива ли нода» / ожидание синхронизации |
| `node config get <путь> [--output table|json|yaml]` | Прочитать значение из config/app.toml |
| `node config set <путь> <значение>` | Изменить значение |
| `node config diff [--output table|json|yaml]` | Сравнить конфиг с рекомендациями (роль авто-определяется) |
| `node peers list [--output table|json/yaml] / add [--persistent/--unconditional] [-y] / remove <node-id|[node-id]@host:port> [-y] / clear [-y]` | Управление persistent-пирами (см. [справочник флагов](#справочник-флагов-команд)). `remove` принимает `<node-id>` или `<node-id>@<host>:<port>`. `add/remove/clear` поддерживают `-y/--yes`. Теперь используют точное совпадение, а не подстроку. |
| `node logs [--level --module --since --tail -f]` | Просмотр и фильтрация логов |
| `node prune [--keep-recent N] [--yes/-y] [--dry-run]` | Обрезка старых блоков и состояния (подтверждение; `--yes` для CI). `--dry-run` показывает что бы произошло. |
| `node snapshot create / list [--output table|json|yaml] / restore` | Управление снапшотами для state sync. `list -o json` теперь возвращает структурированные данные (height/format/chunks). |
| `node statesync enable / disable / show` | Включить/выключить/показать быструю синхронизацию. `show` отображает текущие настройки. |
| `node upgrade info [--output table|json|yaml] / prepare` | Версия бинарника (`info`); `prepare` не реализован — вернёт ошибку. |
| `node keys list / show / export / delete [--keyring-pass] [--yes/-y]`<br>`node keys import <name> [--keyring-password-file/stdin/exec]` | list/show/export/delete требуют `--keyring-pass` (нет алиаса `-p`; `-p` закреплён за `--keyring-password-file` в setup-командах); `export` выводит **unarmored hex** (небезопасно); `import` из armored JSON — пароль через secret‑источники или prompt; `delete` с подтверждением (`--yes` для CI) |
| **Схема паролей keyring** | Два именования — не баг: **setup-фаза** (`init`, `setup keys add`, `keys import`) использует `--keyring-password-{file,stdin,exec}` / `--auto-password`; **runtime-операции** (`node keys list/show/export/delete`) — `--keyring-pass`. Оба источника — см. [«Конфигурация и секреты»](#5-конфигурация-и-секреты) |

### `ops` — обслуживание

| Команда | Зачем |
| ------- | ----- |
| `ops backup [--output <dir>] [--role]` | Бэкап ключей и конфигов (см. §11 — keyring в бэкап не входит!). Роль авто-определяется |
| `ops restore --from <dir> --role <роль> [--yes/-y]` | Восстановление из бэкапа (контейнер должен быть остановлен). Восстанавливает genesis.json и пишет .node-info с chain_id (подтверждение; `--yes` для CI) |
| `ops doctor [--check <имя>] [--all] [--min-ram-mb <n>] [--min-disk-gb <n>] [--role] [-o/--output table\|json\|yaml]` | Диагностика хоста и ноды (роль авто-определяется). `--all` — все проверки (+container-health, wasmvm); `--check <name>` — точечно; `-o json/yaml` — машиночитаемо, exit != 0 при `fail`. Работает с **запущенным** контейнером (host `docker inspect`) |
| `ops verify [--role] [--cross-role]` | Проверка роли и связности (роль авто-определяется) |

### Справочник флагов команд

Ниже — полный перечень флагов, которые принимает каждая подкоманда. Глобальные
флаги (`--container`, `--rpc-url`, `--home`, `--image`, `--data-dir`, `--verbose`)
 описаны в [§14](#14-глобальные-флаги-и-переменные-окружения) и здесь не повторяются.
Кратко: `-o` = `--output`, `-y` = `--yes` (короткие алиасы, работают на всех
соответствующих командах).

> Флаги вида `--<name> (overrides config)` переопределяют поле из YAML-конфига
> (см. [§5](#5-конфигурация-и-секреты)); роль берётся из позиционного аргумента
> `init <role>` / `.node-info`, а не из флага `--role`.

#### `init <role>`

| Флаг | По умолчанию | Описание |
| --- | --- | --- |
| `--config <file>` | (нет) | Typed YAML configuration file (**обязательно**) |
| `--force` | `false` | Переинициализировать, даже если `genesis.json` существует (контейнер должен быть **остановлен**) |
| `--keep-keys` | `false` | Сохранить `node_key.json` + `priv_validator_key.json` при `--force` (удобно для реинициализации sentry без смены NodeID) |
| `--auto-password` | `false` | Сгенерировать случайный пароль keyring и сохранить в `~/.config/umesh/keyring.pass` (XDG config dir, 0600) |
| `--keyring-password-file <f>` | — | Пароль из файла |
| `--keyring-password-stdin` | `false` | Пароль через stdin |
| `--keyring-password-exec <cmd>` | — | Выполнить команду и взять пароль из stdout |
| `--chain-id <id>` | — | Chain ID (overrides config) |
| `--moniker <name>` | — | Моникер (overrides config) |
| `--denom <denom>` | — | Стейк-деном (overrides config) |
| `--min-gas-price <price>` | — | Минимальная цена газа (overrides config) |
| `--environment <env>` | — | Сеть: `mainnet`/`testnet`/`dev` (overrides config) |
| `--pruning <strategy>` | — | `custom\|everything\|default\|nothing` (overrides `node.pruning`) |
| `--genesis-url <url>` | — | URL `genesis.json` для входа в сеть (overrides config) |
| `--sentry-rpc <url>` | — | RPC адрес sentry (для sentry/rpc) |
| `--validator-rpc <url>` | — | RPC адрес валидатора (для sentry) |
| `--rpc-upstream <url>` | — | Upstream RPC для публичного rpc-узла |
| `--rest-upstream <url>` | — | Upstream REST для публичного rpc-узла |
| `--p2p-upstream <addr>` | — | Upstream p2p для публичного rpc-узла |
| `--seeds <addr,...>` | — | Seed-пиры (overrides config) |
| `--persistent-peers <addr,...>` | — | Persistent-пиры (overrides config) |
| `--external-address <ip:port>` | — | Внешний p2p-адрес (overrides config) |
| `--external-port <port>` | — | Внешний p2p-порт (overrides config) |
| `--public-ip <ip>` | — | Публичный IP для sentry (overrides config) |
| `--use-private` | `false` | Зарегистрировать валидатора как private peer (sentry) |
| `--addrbook-url <url>` | — | URL `addrbook.json` |
| `--addrbook-sha256 <hash>` | — | SHA-256 `addrbook.json` для проверки |
| `--genesis-sha256 <hash>` | — | SHA-256 `genesis.json` для проверки |

> `--keep-keys` требует `--force`. `--force` отказывается работать, если контейнер запущен.

#### `validator create`

| Флаг | По умолчанию | Описание |
| --- | --- | --- |
| `--key-name <name>` | `validator` | Имя ключа в keyring |
| `--keyring-pass <pass>` | — | Пароль keyring (prompt, если пусто) |
| `--from <addr>` | — | Адрес делегатора |
| `--moniker <name>` | — | Моникер валидатора |
| `--stake-amount <n>` | `5000000` | Сумма делегации |
| `--denom <denom>` | `uumesh` | Деном стейка |
| `--chain-id <id>` | — | Chain ID |
| `--ip <ip>` | — | Внешний IP (для gentx) |
| `--output <dir>` | `./gentx` | Каталог для gentx |
| `--commission-rate <0..1>` | `0.10` | Комиссия |
| `--commission-max-rate <0..1>` | `0.20` | Макс. комиссия |
| `--commission-max-change-rate <0..1>` | `0.01` | Макс. шаг комиссии |
| `--min-self-delegation <n>` | `1` | Минимум self-delegation |
| `--gas-prices <price>` | `0.0025<denom>` | Цена газа |
| `--gas-adjustment <x>` | `1.5` | Коэффициент газа |
| `--gas-limit <n>` | (авто) | Газ-лимит |
| `--pubkey <key>` | (авто) | Консенсусный публичный ключ |
| `-y` / `--yes` | `false` | Пропустить подтверждение |

#### `genesis add-account`

| Флаг | По умолчанию | Описание |
| --- | --- | --- |
| `--key-name <name>` | — | Ключ (создастся, если нет) |
| `--address <addr>` | — | Существующий адрес (вместо создания ключа) |
| `--mnemonic <phr>` | — | Восстановить ключ из мнемоники |
| `--type <t>` | `base` | `base\|delayed_vesting\|continuous_vesting\|module_account` |
| `--amount <n>` | — | Сумма с деном (напр. `5000000uumesh`) |
| `--start-time <RFC3339>` | — | Точка старта вестинга (для vesting) |
| `--end-time <RFC3339>` | — | Конец вестинга |
| `--module-name <m>` | — | Имя модуля (только для `module_account`) |
| `--keyring-password-{file,stdin,exec}` | — | Источник пароля keyring |

#### `genesis set-param` / `genesis set-time`

| Флаг | По умолчанию | Описание |
| --- | --- | --- |
| `--path <a.b.c>` | — | Точечный путь в genesis.json |
| `--value <json>` | — | Значение (JSON) для `--path` |
| `--time <RFC3339>` | — | Время старта сети |

#### `ops doctor`

| Флаг | По умолчанию | Описание |
| --- | --- | --- |
| `--check <name>` | — | Одна проверка: `arch/ntp/gitignore/readiness/wasmvm/container-health/peers/p2p/join` |
| `--all` | `false` | Все проверки (включая container + wasmvm) |
| `--min-ram-mb <n>` | `4096` | Мин. ОЗУ (0 = выкл) для `arch` |
| `--min-disk-gb <n>` | `50` | Мин. диск (0 = выкл) для `arch` |
| `--role <r>` | (авто) | Роль (validator/sentry/rpc) |
| `-o` / `--output` | `table` | `table\|text\|json\|yaml\|yml` |

#### `ops backup` / `ops restore`

| Флаг | По умолчанию | Описание |
| --- | --- | --- |
| `--output <dir>` | `./backups` | Куда писать бэкап (`backup`) / `--from` для restore |
| `--role <r>` | (авто) | Роль |
| `--from <dir>` | — | Откуда восстанавливать (restore) |
| `-y` / `--yes` | `false` | Пропустить подтверждение |

#### `node logs`

| Флаг | По умолчанию | Описание |
| --- | --- | --- |
| `--level <lvl>` | — | `error\|warn\|info\|debug` |
| `--module <name>` | — | Фильтр по модулю |
| `--since <dur>` | — | Напр. `1h`, `30m` |
| `--tail <n>` | `0` | Последние *n* строк (0 = все) |
| `-f` / `--follow` | `false` | Следить за логами |

#### `node peers` / `node prune` / `node snapshot` / `node statesync` / `node upgrade`

| Команда | Флаг(и) | По умолчанию | Примечание |
| --- | --- | --- | --- |
| `peers add <id>@<ip>:port` | `--persistent` | `true` | Добавить в persistent_peers |
| `peers add <id>@<ip>:port` | `--unconditional` | `false` | Добавить в unconditional_peer_ids |
| `peers add/remove/clear` | `-y`/`--yes` | `false` | Без подтверждения |
| `peers list` | `-o`/`--output` | `table` | `table\|text\|json\|yaml\|yml` |
| `prune` | `--keep-recent <n>` | `0` | Сохранить последние *n* блоков |
| `prune` | `-y`/`--yes` | `false` | Без подтверждения |
| `snapshot create` | `--output <dir>` | `./snapshots` | Куда архивировать |
| `snapshot list` | `-o`/`--output` | `table` | `table\|text\|json\|yaml\|yml` |
| `snapshot restore` | `--from <dir>` | — | Откуда восстанавливать |
| `snapshot restore` | `-y`/`--yes` | `false` | Без подтверждения |
| `statesync enable` | `--trust-height <n>` | `0` | Доверенный высотный блок |
| `statesync enable` | `--trust-hash <hash>` | — | Доверенный хеш блока |
| `statesync enable` | `--rpc-servers <urls>` | — | RPC-источники для sync |
| `statesync enable` | `-y`/`--yes` | `false` | Без подтверждения |
| `upgrade info` | `-o`/`--output` | `table` | `table\|text\|json\|yaml\|yml` |
| `upgrade prepare` | `--version <v>` | — | Целевая версия (напр. `v0.2.0`) |
| `upgrade prepare` | `-y`/`--yes` | `false` | Без подтверждения |

#### `node config diff`

| Флаг | По умолчанию | Описание |
| --- | --- | --- |
| `--role <r>` | (авто) | Роль (validator/sentry/rpc/genesis) |
| `-o` / `--output` | `table` | `table\|text\|json\|yaml\|yml` |

### Прочее

| Команда | Зачем |
| ------- | ----- |
| `version [--short]` | Версия и детали сборки |
| `completion <bash\|zsh\|fish\|powershell>` | Сгенерировать скрипт автодополнения |


## 14. Глобальные флаги и переменные окружения

### Флаги

| Флаг | По умолчанию | Описание |
| --------------- | --------------------------- | ------------------------------------------- |
| `--container` | `umesh-validator` | Имя Docker-контейнера |
| `--rpc-url` | `http://127.0.0.1:26657` | URL RPC-эндпоинта |
| `--home` | `/home/umesh/.umeshnode` | Путь home ноды внутри контейнера |
| `--image` | `umesh-node:latest` | Образ контейнера для офлайн‑операций (контейнер остановлен); для `ops doctor` и `node status docker` используется запущенный контейнер, локальный образ не нужен |
| `--data-dir` | auto | Host-каталог данных (auto: `./data-validator` \| `./data-sentry` \| `./data-rpc`, by role) |
| `--verbose` | `false` | Подробный вывод (debug-логи: data-dir, home, image) |
| `--quiet` | `false` | Подавить не-essential вывод (только ошибки) |
| `--no-color` | `false` | Отключить цветной вывод (также учитывается `NO_COLOR`) |

### Переменные окружения

| Переменная | Описание |
| ---------- | -------- |
| `UMESH_HOME` | Host-каталог данных ноды для автодетекта роли/данных, когда `--data-dir` не задан; по умолчанию перебор `./data-validator`, `./data-sentry`, `./data-rpc` |
| `NODE_IMAGE` | Образ контейнера для офлайн-операций (по умолчанию `umesh-node:latest`; тот же источник, что и `docker-compose.yml`) |
| `SENTRY_RPC` | URL sentry-ноды для `ops verify --cross-role` (приоритет: `--sentry-rpc` > `SENTRY_RPC` > `SENTRY_RPC_FALLBACK` > `--rpc-url`) |
| `SENTRY_RPC_FALLBACK` | Запасной URL sentry для `ops verify --cross-role`, если `SENTRY_RPC` не задан |

### Соответствие полей YAML и node-конфигурации

`umeshctl init` переводит поля YAML-конфига в `config.toml` / `app.toml` /
`genesis.json` контейнера. Из-за разного соглашения об именовании (camelCase в
YAML vs snake_case/kebab-case в TOML) удобно держать таблицу под рукой — особенно
при ручной правке `config.toml` через `node config set`/`get`:

| YAML (camelCase) | node-конфиг | Файл |
| --- | --- | --- |
| `node.dataDir` | (host-каталог, не в TOML) | docker-compose `.env` / `--data-dir` |
| `node.moniker` | `moniker` | `app.toml` |
| `node.environment` | (выбирает `.env.<environment>`; не пишется в TOML) | docker-compose |
| `node.pruning` | `pruning`, `pruning-interval` | `app.toml` (значения: `custom/everything/default/nothing` совпадают) |
| `chain.chainId` | `chain_id` | `config.toml` / `genesis.json` |
| `chain.denom` | `bond_denom` | `genesis.json → app_state.staking.params.bond_denom` |
| `chain.minGasPrice` | `minimum-gas-prices` | `app.toml` |
| `network.seeds` | `p2p.seeds` | `config.toml` |
| `network.persistentPeers` | `p2p.persistent_peers` | `config.toml` |
| `network.externalAddress` | `p2p.external_address` | `config.toml` |
| `network.usePrivate` | `p2p.private_peer_ids` (флаг-переключатель) | `config.toml` |
| `network.externalPort` | (port mapping) | docker-compose `ports` |
| `network.publicIp` | (вычисляется → `p2p.external_address`) | config.toml |

Справка по отдельным полям: `umeshctl node config diff` уже сравнивает
`p2p.seeds`, `p2p.persistent_peers`, `p2p.external_address`, `rpc.laddr`,
`statesync.enable`, `api.enable` с рекомендациями — это быстрая проверка
соответствия вашей руки конфигурации и кода.

> Каждый тип ноды (validator/sentry/rpc) обычно живёт в своём Docker-контейнере
> на своём VPS. `p2p.external_address` обязателен: без него нода в контейнере
> анонсирует внутренний bridge-IP и недоступна для пиров с других хостов. RPC
> валидатора публикуется наружу (`VALIDATOR_RPC_BIND_IP=0.0.0.0`) — Sentry и
> RPC-ноды подключаются к нему по `http://<validator-ip>:26657`.


## 15. Частые ошибки и диагностика

| Симптом | Вероятная причина | Что делать |
| ------- | ------------------ | ---------- |
| `Preflight: image not found` | Образ `umesh-node:latest` не собран | Соберите образ в репозитории `Node_Umesh`: `docker build -t umesh-node:latest -f Dockerfile .` |
| `unknown role "X"` / `role is required` | Указана неверная роль или роль не передана в `init <role>` | Допустимые роли: `genesis`, `validator`, `sentry`, `rpc` |
| `already initialized` (предупреждение, не ошибка) | Повторный запуск `init <role>` без `--force` для уже проинициализированной ноды | Это ожидаемое поведение (идемпотентность). Если нужно пересоздать — `--force` (контейнер должен быть остановлен) |
| `refusing --force while container is running` | Попытка `--force` на запущенном контейнере | Сначала `docker compose down` |
| `container "..." is not running — start node first` | Онлайн-команда (например, `validator create`) вызвана при остановленном контейнере | Запустите контейнер: `docker compose --env-file .env.<role> --profile <role> up -d` |
| `failed to download genesis from N source(s): tried [url: err, url: err, ...]` | Ни один источник `join.*` не отвечает | Проверьте каждый URL вручную через `curl`, используйте `umeshctl genesis fetch --dry-run` для диагностики без записи на диск |
| `insufficient balance: has X, need Y (delegation + fee)` | На кошельке валидатора не хватает `amount + расчётная комиссия` (`0.0025 × 300000 = 750` в базовой деноминации по умолчанию) | Пополните баланс и повторите `validator check-balance` перед `validator create` |
| `node is not fully synced … wait: umeshctl node health --wait-sync` | Регистрация валидатора запущена до окончания синхронизации | Дождитесь `catching_up: false`, используйте `--wait-sync --timeout` |
| Внешний IP анонсируется как `172.x` | `network.externalAddress` не задан или задан неверно | Пропишите реальный публичный `IP:26656` в YAML или через `node config set p2p.external_address` |
| Валидатор ушёл в jail | Долгий даунтайм / double-sign / закрытый порт 26656 не давал подписывать блоки | `validator signing-info` — посмотреть причину; если не tombstoned — `validator unjail` |
| Валидатор в tombstone | Зафиксирован double-sign | Необратимо на уровне протокола — восстановить того же валидатора в сети нельзя, нужен новый оператор/ключ |

**Главные предостережения, которые стоит держать в голове всегда:**

- Никогда не восстанавливайте `priv_validator_state.json` на уже работающем
  валидаторе.
- Никогда не запускайте `--force` при запущенном контейнере ноды.
- `ops backup` не копирует `keyring/` — бэкапьте его отдельно (см. §11).
- Файрвол — не забота `umeshctl`: открытые/закрытые порты нужно проверять
  вручную (`ss`, `nc`, `ufw`/`iptables`).
