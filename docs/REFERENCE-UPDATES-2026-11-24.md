# Проверка референсов 24 ноября 2026 года

Сравниваются коммиты, деревья пакетов и конкретные файлы, а не только номера
версий. Исходные pins в `source-lock.json` не заменяются коммитом, из которого
ничего не переносилось. Для уже опубликованных релизов (`devin` `0.8.3`,
`minimax` `0.1.1`, `opencode` `0.8.5`, а также неизменённый `omp` `0.1.0`)
поставляемые файлы пакетов не затрагиваются; там, где подтверждено отсутствие
дефекта, добавлены только `*_test.go` регрессии (не входящие в release inputs).

| Наш пакет | Версия | Дополнительно проверено | Результат сравнения |
| --- | --- | --- | --- |
| [Command Code](../packages/commandcode/README.md) | `0.1.6` | Community `0.1.24` (`e967ba4d`), npm `command-code` `1.93.3`, `pi-commandcode-provider` `0.8.7` (`a063270a`), `CLIProxyAPI` `85225381` | Community-дерево не изменилось (`305e867f`). `cliVersion` (`x-command-code-version`) обновлён `1.91.0 -> 1.93.3` для native Generate и System One; 9 wire-функций и `dist/index.mjs` побайтово совпадают. `generate_stream.go` уже отклоняет `"abort"` без `"finish"`; `"zdr": true` в BYOK `providers.json` не фабрикуется |
| [Devin](../packages/devin/README.md) | `0.8.3` | Community `0.1.45` (`e967ba4d`), `Agents/devinapp` `5f067fdf`, `CLIProxyAPI` `85225381` | Community-дерево не изменилось (`1c23cc5c`). `devinapp` `91b53667` (`3000.14.30821` live capture) и `89f2cb47` (`0x02` error trailer после data-кадров + nested/flat JSON) уже покрыты в `codec`; добавлен регрессионный тест `codec/reference_20261124_test.go`. Расхождения CLIProxyAPI (`cb5f62f7` OpenAI Responses, `a516bf3b` lower-effort tie-break) не переносятся |
| [MiniMax](../packages/minimax/README.md) | `0.1.1` | Community `0.2.9` (`e967ba4d`), `MiniMax-AI/minimax-code` `3c21163c` | Полное совпадение дерева `098cf5e6` и HEAD референса `3c21163c`. Перенос не требуется |
| [OMP](../packages/omp/README.md) | `0.1.0` | `can1357/oh-my-pi` `692470ba` (`19.1.1+`), `openai/codex` `9202c791`, `codex-infinity` `dfd6df91` | В `replay-policy.ts` и `compaction/messages.ts` 0 коммитов. Аддитивные поля `19.1.1+` (`origin: "chrome"`, `role: "temporary"`, `fallback`, `model_usage.{duration,ttft,judgment,completion}`, `tan-fork-boundary`) безопасно игнорируются; `plan-mode-context` уже исключён allowlist-ом `custom_messages.go`. Добавлен тест `reference_20261124_test.go` |
| [OpenCode](../packages/opencode/README.md) | `0.8.5` | `anomalyco/opencode` `24449184` (`2.4.1`), Community `zen-free` `0.1.28` (`e967ba4d`), `yetone/magpie` `e6f4af3f` | 0 коммитов во всех отслеживаемых файлах SQLite/credentials/config. `packages/core/src/usage-limits.ts` по-прежнему вызывает `/zen/go/v1/usage`; `projectLive` уже фильтрует `type != "chat"` и `retired()`. Добавлен тест `reference_20261124_test.go` |
| [ZCode](../packages/zcode/README.md) | `0.16.1` | `zai-org/ZCode` `a4b44451` (desktop `4.0.0` / CLI `0.18.0`), Community `0.1.69` (`e967ba4d` / `fdbf261b`) | Community `0.1.69` (`ee4de91f`) уже портирован; `origin/zcode-cli` (`bc99797f`) — отдельная ветка (наблюдение). Из `4.0.0` портирован `IsProviderCodeShaped` (`^[A-Za-z0-9_.:-]{1,64}$`) в `protocol/response.go` и `request_policy.go` с тестами. `ClientVersion` остаётся `3.18.2` до полного захвата wire `4.0.0` |
| [CodeArts](../packages/codearts/README.md) | `0.1.0` | Community `0.3.22` (`96467464` / C33 `818b4b65`, upstream `e967ba4d`) | Новый нативный SDK-порт, соответствует актуальному дереву `0f0764c1` |
| [Kimi Work](../packages/kimi-work/README.md) | `0.1.0` | Community `0.1.17` (`e967ba4d`) | Новый нативный SDK-порт, соответствует актуальному дереву `d57dc1b1` |
| [Qoder](../packages/qoder/README.md) | `0.1.0` | Community `0.2.73` (`e967ba4d`) | Новый нативный SDK-порт, соответствует актуальному дереву `45397b48` |
| [WorkBuddy](../packages/workbuddy/README.md) | `0.1.0` | Community `0.2.37` (`96467464` C33, upstream `e967ba4d`) | Новый нативный SDK-порт, соответствует актуальному дереву `4683e868` |

## Community-доноры (`magpie-community/plugins`)

Upstream `https://github.com/magpie-community/plugins` (`origin/main`) находится
на коммите `e967ba4d946071c6c27a51cd5168b12ce832da5c`; локальная ветка `main`
содержит два локальных C33-коммита (`818b4b65f88751596a2768ea37f443d23201b236`
для CodeArts `0.3.22` и `964674642899f4827ed3d38e4a91fdc344e6448c` для
WorkBuddy `0.2.37`).

- `packages/commandcode` (`0.1.24`, дерево `305e867f7f675041d4905c951fb2495a9d00fcb3`):
  без изменений.
- `packages/devin` (`0.1.45`, дерево `1c23cc5c881c36b027f8cef2feb70f7a49ada5e4`):
  без изменений.
- `packages/minimax` (`0.2.9`, дерево `098cf5e689d6df7e675b7b5ef81c9e4780fa4bc7`):
  без изменений.
- `packages/zen-free` (`0.1.28`, дерево `84f1e08de8e6b72ea3ecfe1ccc324476280ab287`):
  без изменений.
- `packages/zcode` (`0.1.69`, `fdbf261b7d01d1c908220029473d671cc420cab7`,
  дерево `ee4de91f09b88382e6b2d55236b1f1e3e93dc6ec`): уже перенесён в
  `packages/zcode` (`community-zcode-0169`). Ветка `origin/zcode-cli`
  (`bc99797f`) — отдельный headless-плагин и не является заменой нашего модуля.
- Новые порты `kimi-work` (`0.1.17`, `d57dc1b1`), `codearts` (`0.3.22`,
  `0f0764c1`), `workbuddy` (`0.2.37`, `4683e868`) и `qoder` (`0.2.73`,
  `45397b48`) совпадают с текущими срезами доноров.

## Официальные и сторонние референсы

### Command Code (`command-code` `1.93.3`, `pi-commandcode-provider` `0.8.7`)

- npm-архив `command-code@1.93.3` проверен без запуска CLI:
  SHA-1 `eb794928692586be814036c65b643eb4c37c9b7b`,
  SHA-256 `73d27cc2f0f86d44eabcbf889a7edc9914f168f4e426435540c053e45b68a07c`,
  integrity `sha512-iupga0dt7QghDZO0hMYMhGDXj0dJCWFQ8xBn4+YR1602JN0q7E3mY91NOlaa4W14p9pLHogpQZ3YAgn/tJzd1g==`.
- `dist/index.mjs` (`157feefa...`) и 9 выбранных wire-функций (`toWireTools`,
  `toWireToolOutput`, `toWireMessages`, `toWireSystem`, `readStreamErrorEvent`,
  `normalizeStopReason`, `addUsage3`, `reasoningProviderOptions`,
  `toWirePermissionMode`) побайтово совпадают с `1.91.0`. Транспортные
  константы (`/alpha/generate`, `64000`, `x-cmd-zdr`, `x-command-code-version`,
  `pause_turn`) без изменений.
- `createModelClient` добавил только клиентскую телеметрию заголовков ответа
  (`onResponseHeaders`). `consumeStream` начал выбрасывать `AbortError` при
  `"abort"` без `"finish"`, но `packages/commandcode/src/generate_stream.go:286`
  уже завершает поток ошибкой на `"abort"`.
- В `1.93.3` появился opt-in флаг `"zdr": true` для BYOK-записей в
  `~/.commandcode/providers.json`. Наш агентный планировщик (`agent.go`) не
  добавляет `"zdr": true` безусловно, чтобы не заявлять ZDR для произвольного
  шлюза Magpie.
- Портировано: `cliVersion = "1.93.3"` (версия пакета поднята `0.1.5 -> 0.1.6`)
  в `module.go`, обновлены проверки заголовков в `module_test.go` и
  `systemone_test.go`, добавлена запись `referenceReview20261124` в
  `packages/commandcode/docs/source-lock.json`.
- `patlux/pi-commandcode-provider` продвинулся от `6fd0ac75` (`0.7.1`) до
  `a063270a733768eea5d2fb56edc0c8c081308e34` (`0.8.7`, 95 коммитов): в `0.8.0`
  удалён fallback `/alpha/generate` (`src/json-schema.ts`/`src/converters.ts`),
  добавлена совместимость Responses и синхронизация каталога `1.93.2`. Для
  нашего native Go-plan правило `geminiNullableTools` остаётся актуальным.
- `router-for-me/CLIProxyAPI` (`061ffe6f -> 85225381d89915595598ed13766cd79890c03a84`):
  0 коммитов по `commandcode*`.

### ZCode (`zai-org/ZCode` `4.0.0` / CLI `0.18.0`)

- Официальный репозиторий продвинулся от `fec97590` (desktop `3.18.2` / CLI
  `0.17.1`) до `a4b4445141eb7d3b3e99eada9292775383bfc44e` (desktop `4.0.0` /
  CLI `0.18.0`, 1 sync-коммит, 2209 файлов). Поправка OAuth clock skew из
  `oauthService.ts` и regexp модели revision 44 уже были перенесены в `0db332b`.
- Проверены 44 изменившихся файла в ключевых подсистемах:
  - SQLite (`migrations.ts`, `schema.baseline.sql`): добавлен только индекс
    `0026_dwf_run_parent_session_index` по `dwf_run(parent_session_id)`,
    чтение сессий и сообщений не затронуто.
  - `zcode-protocol-v4` / `zcode-protocol`: аддитивные необязательные поля и RPC
    (`sessionDeletionPreviewBatch`, `turnAdmissionTelemetry`, `workspaceTrust`,
    `regions`, `ZCODE_PROVIDER_PROJECT_TOKEN_NETWORK_ERROR = -32033`).
  - `shared-credentials.ts` / `systemCredentialCipher.ts`: генерационный барьер
    записи и `ZCODE_CREDENTIALS_FILE` относятся к локальному desktop/CLI
    хранилищу, к которому наш модуль не обращается.
  - `zcode-builtin-remote-synchronizer.ts`: очередь `force`-обновлений и
    допуск переиспользования CDN-URL — клиентская синхронизация.
  - `withOpenAiResponsesReplayOptions` в `model-execution.ts` относится к
    OpenAI-адаптеру самого ZCode CLI, а не к нативному Messages-маршруту ZCode.
- Портировано: `IsProviderCodeShaped` (`^[A-Za-z0-9_.:-]{1,64}$`) из
  `model-execution.ts`, `failure-classifier.ts` и
  `provider-finish-business-error.ts` перенесён в
  `packages/zcode/src/protocol/response.go` и
  `packages/zcode/src/request_policy.go` с тестами в `response_test.go` и
  `request_policy_test.go`. HTML/скриптовые и чрезмерно длинные значения поля
  `code` (например, в HTTP 499 от прокси) больше не попадают в `Facts.Code` и не
  вызывают ложную классификацию бизнес-ошибки. Зафиксировано в
  `packages/zcode/docs/source-lock.json` (`vendor-zcode-provider-code-shape`) в
  рамках ещё не опубликованной версии `0.16.1`.
- `ClientVersion` остаётся `"3.18.2"` (`vendor-current-audit`) до независимого
  аудита полного wire-захвата desktop `4.0.0`.

### OpenCode (`anomalyco/opencode` `2.4.1`)

- Репозиторий продвинулся от `011f9043` (`2.3.0`) до
  `244491841dea5735c4c069ddc217e1e5f16126bd` (`2.4.1`, 368 коммитов).
- Все 5 отслеживаемых файлов SQLite-сессий (`sql.ts`, `history.ts`,
  `stored.ts`, `blob.ts`, `instruction-state.ts`), а также `database-path.ts`,
  `global-roots.ts`, `console-access.ts`, `credential.ts`,
  `schema/config/provider.ts`, `v1/config/provider-options.ts` и
  `database/v1-migration.bun.ts` имеют 0 коммитов.
- Endpoint usage для Go перепроверен в `packages/core/src/usage-limits.ts`
  (`e42f6d10`): официальный CLI по-прежнему запрашивает `${origin}/zen/go/v1/usage`
  с `Bearer` и `x-opencode-org-id`. Коммит `93a1ed0d` (`go.mdx`) документирует
  публичный алиас `/inference/go/v1/usage` с той же JSON-схемой
  `usage.{rolling,weekly,monthly}`, которую `ParseUsage` уже разбирает.
- Обновления каталога моделей (`71cd8c06` add `longcat-2.5` / deprecate
  `longcat-2.0` и `longcat-2.5-free`, `b104f977` deprecate `inkling-2-small-free`,
  `a9156f58` add `hungerg-free`, `c94ff33f` add `kimi-k3.1-pro`, `33d22fe2` add
  `jev-1.14`, `81045bf4` Go decision и embedding модели) динамически покрываются
  `projectLive` (`model.Type != "chat" || model.retired()`).
- Коммит `8bf09030` (`connect OpenCode Go through the OpenCode connection`)
  меняет только UI-диалог и локализацию в `packages/app`, не затрагивая схему
  конфигурации агента.
- `yetone/magpie` (`99cc8fa5 -> e6f4af3f`): без OpenCode-коммитов
  (`fd166b280` касается встроенного omp/OmO Gemini relay, `fb1041a07` добавляет
  заголовки caller в хосте).
- Код опубликованного релиза `0.8.5` не изменялся; добавлен регрессионный тест
  `packages/opencode/src/reference_20261124_test.go`.

### Devin (`Agents/devinapp` `5f067fdf`, `CLIProxyAPI` `85225381`)

- Локальный `Agents/devinapp` продвинулся от `12bde35f` до
  `5f067fdfbffaff8652b668419c0e3f1b2d1866bf`. Большинство изменений относится к
  permission engine (`affogato`/`barista`) и локальным инструментам.
  - `91b53667` (`live-gcm-1`): живой захват `3000.14.30821` `GetChatMessage`
    подтверждает минимальный набор полей (`f1`, `f3`, `f7=5`, `f8`, `f20=1`,
    `f21`, gzip `0x01`), уже реализованный нашим кодеком.
  - `89f2cb47` (`gcm-trailer-fix-1`): Connect EndStream `0x02`
    `{"error":{"code","message"}}` после data-кадров и разбор nested/flat JSON
    уже реализованы в `codec/outbound_response.go`, `codec/paired.go` и
    `codec/response_failure.go` (`BeforeEffect` сбрасывается после первого
    data-кадра). Добавлен регрессионный тест
    `packages/devin/src/codec/reference_20261124_test.go`.
- `router-for-me/CLIProxyAPI` (`061ffe6f -> 85225381d89915595598ed13766cd79890c03a84`):
  - `cb5f62f7` (`preserve gpt reasoning across responses tool calls`) касается
    OpenAI Responses в трансляторе CLIProxyAPI; наш модуль `devin-inbound`
    декларирует `chat`, `anthropic` и парный native `GetChatMessage` (без
    OpenAI Responses) и уже защищает подписи reasoning.
  - `a516bf3b` (`use canonical thinking clamping`) меняет tie-break в
    CLIProxyAPI на меньший уровень (`swe-2` `xhigh -> high`, `glm-5-3`
    `medium -> low`). Наш модуль сохраняет tie-up паритет с донором
    `magpie-community/plugins` `0.1.45` (`alias-nearest-tie-up`).
- Опубликованный релиз `0.8.3` не изменялся.

### OMP (`oh-my-pi` `692470ba`, `openai/codex` `9202c791`, `codex-infinity` `dfd6df91`)

- `can1357/oh-my-pi` продвинулся от `04a4c101` (`18.22.2`) до
  `692470bad9fe487af28c5475b7984f9b4d8c37b5` (`19.1.1+`, 1836 коммитов).
  `replay-policy.ts` и `compaction/messages.ts` — 0 коммитов. Все 6 коммитов
  по `session-entries.ts` и `session/messages.ts` (`8cc8bcdf`, `c82aee45`,
  `9e0442f0`, `d6e952c9`, `c228352c`, `32707b85`) добавляют необязательные
  поля (`origin: "chrome"`, `role: "temporary"`, `fallback`, `duration`/`ttft`/
  `judgment`/`completion`, `tan-fork-boundary`, `errorFromAgentLoop`) или
  исключают `plan-mode-context` (который `custom_messages.go` уже отфильтровывает
  allowlist-ом `collab-prompt`/`skill-prompt`).
- `openai/codex` (`6d9f0199 -> 9202c7916b434d5312ac580148b7d2460e57fe7e`, 379
  коммитов): проверены все 6 коммитов rollout/history; `77acaa20` сохраняет
  дисковый формат `{ "type": "event_msg", "payload": ... }`, а `bd00a8a4` /
  `ab7a9377` относятся к realtime/Guardian истории.
- `lee101/codex-infinity` (`229e8898 -> dfd6df915b77377c231c62365e8d0545d7b5c57e`):
  все 12 коммитов по отслеживаемым путям — слияния upstream `openai/codex`.
- Релизные байты `0.1.0` не изменялись; добавлен регрессионный тест
  `packages/omp/src/reference_20261124_test.go`.

### MiniMax (`minimax-code` `3c21163c`)

- `MiniMax-AI/minimax-code` HEAD совпадает с `3c21163c85a9684030cb147bfef48c6284fa3c46`,
  а `packages/minimax` в `magpie-community/plugins` (`0.2.9`, дерево `098cf5e6`)
  не изменился. Релиз `0.1.1` не затрагивался.

## Границы проверки

Все проверки выполняются офлайн на `usbox` (`linux/amd64`, закреплённый Go
`1.27.4`, изолированные `HOME`/`XDG_*`, `GOPROXY=off`) с `scripts/check.py all --race`,
`tests/test_workspace.py`, `scripts/build.py all` и проверкой
`scripts/release_ci.py bumped`. Живые запросы к вендорам, реальные учётные
записи, нативные macOS/Windows сборки и запуск установленных CLI-клиентов в эту
проверку не входят.
