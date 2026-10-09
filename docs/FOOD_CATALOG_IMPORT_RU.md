# Импорт открытого каталога продуктов

Сырые выгрузки обрабатываются на рабочем ПК. На production передаётся только
компактный gzip JSONL с российскими продуктами и КБЖУ. Фотографии не входят в
пакет.

Источник карточек: Open Food Facts, лицензия ODbL 1.0.

Также поддерживается подготовленная выгрузка АШАН. Подготовщик исключает строки
со статусом `invalid`, объединяет товары с одинаковым нормализованным названием,
выбирает запись с лучшим статусом качества и дополняет отсутствующие поля из
дублей. Личные продукты и дневник при импорте не изменяются.

## Подготовка на рабочем ПК

```bash
mkdir -p data/catalog
curl -L -C - -o data/catalog/en.openfoodfacts.org.products.csv.gz \
  https://static.openfoodfacts.org/data/en.openfoodfacts.org.products.csv.gz
make catalog-prepare FILE=./data/catalog/en.openfoodfacts.org.products.csv.gz
sha256sum data/catalog/openfoodfacts-ru.jsonl.gz
```

Для JSON-каталога АШАН:

```bash
make catalog-prepare-auchan FILE=/path/to/auchan_catalog.json
make catalog-import FILE=./data/catalog/auchan-2026-09-07.jsonl.gz
```

Подготовщик потоковый: распакованный многогигабайтный CSV на диск не пишет.

## Production

Скопируйте только `openfoodfacts-ru.jsonl.gz` и запустите:

```bash
make backup
make catalog-import FILE=./data/catalog/openfoodfacts-ru.jsonl.gz
```

Команда выполняет upsert по `(provider, external_id)`. Пользователи, дневники
питания и продукты, созданные вручную, не удаляются.
