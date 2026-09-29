
## Полезные команды

- Документация `openapi`

Спецификация
```bash
npx @redocly/cli bundle > ./api/dist/openapi.yaml 
```

Удобно читаемая документация
```bash
npx @redocly/cli build-docs api/openapi.yaml -o ./api/dist/docs.html
```