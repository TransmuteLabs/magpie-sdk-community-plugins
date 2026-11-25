# Плагины и совместимость

Каждый пакет устанавливается отдельно и содержит собственную реализацию.
Community-плагины и исходники harness служат референсами; дополнительная
установка этих плагинов для работы наших пакетов не требуется.

| Пакет | Версия в исходниках | Минимум API | Исходные и дополнительно проверенные референсы |
| --- | --- | --- | --- |
| [CodeArts](../packages/codearts/README.md) | `0.1.0` | `1.12` + `provider-signin`, `provider-saved-signin`, `request-policy` | Community `0.3.22` (`96467464` / C33 `818b4b65`, upstream `e967ba4d`). CodeArts и OfficeAce, OAuth PKCE/DPoP, HMAC, изоляция `:benefit`, usage и check-in |
| [Command Code](../packages/commandcode/README.md) | `0.1.6` | `1.12` + `provider-signin` | Community `0.1.24` (`e967ba4d`). CLI `1.93.3` сверён с `1.91.0`; Native Generate и System One отправляют `1.93.3`. `pi-commandcode-provider` просмотрен до `0.8.7` (`a063270a`); nullable-схемы Gemini Go сохранены |
| [Devin](../packages/devin/README.md) | `0.8.3` | `1.12` + `provider-signin`, `provider-saved-signin` | Протокол Devin `3000.11.228` сверён на `b86869df`, `Agents/devinapp` просмотрен до `5f067fdf`, `CLIProxyAPI` до `85225381`, Community `0.1.45` на `e967ba4d`. Connect `0x02` trailer после data-кадров и `failed_precondition` покрыты. Живая проверка ожидает сборок |
| [Kimi Work](../packages/kimi-work/README.md) | `0.1.0` | `1.12` + `provider-signin`, `request-policy` | Community `0.1.17` (`e967ba4d`). Отдельный провайдер `kimi-work` по desktop sign-in, слияние каталога, нормализация схем и usage |
| [MiniMax](../packages/minimax/README.md) | `0.1.1` | `1.12` + `provider-signin` | Community `0.2.9` (`e967ba4d`), `minimax-code` `3c21163c` (без изменений на 24 ноября) |
| [OMP](../packages/omp/README.md) | `0.1.0` | `1.10` | OMP `18.22.2` (`04a4c101`), дополнительно проверены `oh-my-pi` `19.1.1+` (`692470ba`), `openai/codex` `9202c791` и `codex-infinity` `dfd6df91`. Экспорт учитывает `custom_message`, исключает `plan-mode-context` и сохраняет архив побайтово |
| [OpenCode](../packages/opencode/README.md) | `0.8.5` | `1.12` + `provider-signin` | Go, Zen и Zen Free подключаются через Providers. Исходные pins `011f9043e8` и `zen-free` `0.1.28` сохранены; `anomalyco/opencode` `2.4.1` (`24449184`) проверен 24 ноября (`/zen/go/v1/usage` и фильтрация non-chat/retired подтверждены) |
| [Qoder](../packages/qoder/README.md) | `0.1.0` | `1.12` + `provider-signin`, `provider-saved-signin`, `request-policy` | Community `0.2.73` (`e967ba4d`). Qoder, Qoder CN, QwenWork и QwenWork Enterprise, PKCE/PAT, enterprise VPC и check-in |
| [WorkBuddy](../packages/workbuddy/README.md) | `0.1.0` | `1.12` + `provider-signin`, `provider-saved-signin`, `request-policy` | Community `0.2.37` (`96467464` C33, upstream `e967ba4d`). WorkBuddy и WorkBuddy AI, OAuth/desktop `.info`, кредиты, 24h free-window и check-in |
| [ZCode](../packages/zcode/README.md) | `0.16.1` | `1.20` + `provider-signin`, `provider-saved-signin` | Desktop `3.18.2` и CLI `0.17.1` на `fec97590`, Community `0.1.69` (`fdbf261b` / `e967ba4d`). Из `4.0.0` (`a4b44451`) перенесены clock skew OAuth и проверка `IsProviderCodeShaped`. Подписки с Desktop-follow остаются за Community |

Версия пакета, минимальная ревизия API и версия референса независимы. Все пакеты используют
native ABI `1`. Точные исходные коммиты, версии, переносимое поведение,
сравнения с обновлёнными референсами и границы проверок записаны в:

- [CodeArts source-lock](../packages/codearts/docs/source-lock.json).
- [Command Code source-lock](../packages/commandcode/docs/source-lock.json).
- [Devin source-lock](../packages/devin/docs/source-lock.json).
- [Kimi Work source-lock](../packages/kimi-work/docs/source-lock.json).
- [MiniMax source-lock](../packages/minimax/docs/source-lock.json).
- [OMP source-lock](../packages/omp/docs/source-lock.json).
- [OpenCode source-lock](../packages/opencode/docs/source-lock.json).
- [Qoder source-lock](../packages/qoder/docs/source-lock.json).
- [WorkBuddy source-lock](../packages/workbuddy/docs/source-lock.json).
- [ZCode source-lock](../packages/zcode/docs/source-lock.json).

[Отчёт за 24 ноября](REFERENCE-UPDATES-2026-11-24.md) и
[отчёт за 17 ноября](REFERENCE-UPDATES-2026-11-17.md) сохраняют сравнения
на дату прогона. Сравнения за 18 ноября записаны в source-lock пакетов
Command Code, OMP и OpenCode. Сравнения локальных референсов и проверки
Devin `0.8.2` и OpenCode `0.8.1` записаны в соответствующих пакетах.
Номера в колонке API обозначают совместимость контракта. Публичный релиз
SDK имеет `VERSION` приложения и не называется `1.12`.

Проверка новых refs сама по себе не означает перенос новой реализации.
Одинаковая версия референса тоже не доказывает одинаковые исходники: сравниваются
коммиты и конкретные пути. Минимум SDK в манифесте означает требуемый контракт;
он не подменяет идентификатор хоста, на котором выполнялись проверки.

## Установка и выпуск

Magpie с SDK может работать с пустым каталогом расширений. Первый перевод
приложения выполняется без внешних плагинов; пакеты добавляются по готовности.

У каждого модуля своя версия и релиз `<module-id>-v<semver>`. Установщик
принимает `github:owner/repository#module-id[@module-id-vSEMVER]`.
Сборка одного пакета не требует пересборки остальных. Проверенное повторное
использование и пропуск уже опубликованных версий описаны в
[общих командах сборки и выпуска](../README.md#release-assets).

21 ноября 2026 года в приватном GitHub Releases подтверждены публикации
`opencode-v0.8.0`, `opencode-v0.8.2` и `opencode-v0.8.3`. В релизе `0.8.3`
есть архив `darwin/arm64` и `checksums.txt`. Установка `0.8.0` по ссылке
на репозиторий и проверка обновления проходили в отдельном профиле macOS.

## Перенос подписки

1. Оставьте Community-плагин установленным и включённым. Выполните
   `magpie plugin sdk auth import` для каждого сохранённого аккаунта.
2. Передайте все действующие билеты в один `magpie plugin sdk auth transfer`.
   Проверьте активную запись через `magpie plugin sdk auth owners`, одну
   подписку под прежним ID, алиасы аккаунтов, Usage и запрос через
   существующий маршрут.
3. При проблемах выполните `magpie plugin sdk auth rollback` с публичным
   ID и ревизией владельца. Откат требует исходных сохранённых записей и
   включённого Community-плагина. Сделайте его до смены credentials.
   После приёмки SDK удалите Community-плагин, если откат больше не нужен.

`auth activate` добавляет отдельный аккаунт под SDK ID и сохраняет
владельца существующей подписки. Для Devin перенос проверен на локальном
mock. Для ZCode записи Desktop-follow остаются у Community, пока нельзя
подтвердить связь личности и ключа. Браузерный вход создаёт отдельный
SDK-аккаунт. [Инструкция Devin](../packages/devin/docs/ONBOARDING.md#import-a-saved-community-sign-in) ·
[Инструкция ZCode](../packages/zcode/docs/REFERENCE.md#settings-panel-and-subscription-sign-in)

## Совместный прогон 19 ноября 2026 года

19 ноября 2026 года все семь пакетов повторно прошли `scripts/check.py all
--race` на `usbox` со снимком Magpie `1b94b2e0b` и незакоммиченными правками
SDK. В прогон вошли даты сброса квот и USD-баланс Command Code, сохранение
хода при rollback Codex в OMP, незавершённые SSE и поздний usage OpenCode
Go/Zen, а также `Retry-After` при тайм-ауте очереди ZCode. Свежая сборка
Magpie на том же снимке стартовала с пустыми каталогами расширений,
записала Usage и завершила потоковый запрос после `SIGTERM`.

Дополнительная регрессия OpenCode проверяет явную ошибку Gemini после
`finishReason` для Go и Zen. Оба варианта ранее завершали такой запрос
успешно. Теперь они возвращают `uncertain`. На `usbox` прошли 20 повторов
регрессии с `-race`, полный пакет с `-race` и `go vet`.

Command Code, Devin, MiniMax, OMP, OpenCode и ZCode также собраны и
установлены по одному на эту сборку Magpie с `CGO_ENABLED=1`. Проверены
хеши установленных файлов, включение, выключение, перезагрузка и удаление
пакетов в чистых профилях. Установка Claude Desktop проверяется на macOS.

## Совместный прогон 18 ноября 2026 года

На `usbox` (`linux/amd64`, Go `1.27.3`) проверены Magpie
`f7bf12c1f6884e4ac60bad1c704c579f5689866c` и плагины
`28cdc1991b26db16d64c20dec587240902a968bf`. Во время прогона все
11 407 файлов Magpie и 915 файлов плагинов сохранили содержимое и признак
исполняемости из архивов этих коммитов.

- Все семь пакетов прошли проверку форматирования, `go vet`, `go test -race` и
  проверки, перечисленные в реестре. [Команда проверки](../scripts/check.py).
- Command Code, Devin, MiniMax, OMP, OpenCode и ZCode собраны и установлены
  по одному через обычный Magpie SDK CLI. Для каждого пакета чистый профиль
  содержал ровно один нативный модуль без legacy-плагинов и `node_modules`.
  Проверены хеши установленных файлов, выключение, включение, перезагрузка
  и удаление. [Сборка](../scripts/build.py), [сценарий установки](../tests/install_packages.py).
- OMP прошёл экспорт и обратный импорт архива с `custom_message` через
  установленный модуль. Исходный журнал и вложение сохранились побайтово.
  [Сценарий `--custom-messages`](../packages/omp/tests/verify_host.py).
- Для прежнего пакета Claude Desktop выполнены проверки исходников на Linux.

Коды завершения, SHA-256 исходников, бинарника Magpie и дистрибутивов записаны
в `usbox:/tmp/magpie-seven-fixtures.5b0KUFRO/receipt.json`. Для воспроизведения
используются указанные коммиты, офлайн-кеши Go, чистые HOME/XDG и сетевой
ограничитель с локальными тестовыми серверами. Живые испытания аккаунтов,
реальных harness и Claude Desktop, macOS Keychain и обновления установленного
приложения ещё предстоят.

## Записи проверок

[Срез от 16 ноября 2026 года](STATUS-2026-11-16.md) сохраняет исходный отчёт
с точными историческими результатами и ограничениями. Его положительные
результаты относятся к указанным срезам, а не к последующим изменениям.
Детальные записи каждого пакета доступны из его README и source-lock.
Claude Desktop подключается через встроенный раздел **Agents → Claude Desktop**.
Отдельный пакет снят после переноса проверки профиля и отката настроек в Magpie.
