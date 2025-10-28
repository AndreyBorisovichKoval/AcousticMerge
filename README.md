[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[![Release](https://img.shields.io/github/v/release/AndreyBorisovichKoval/AcousticMerge)](https://github.com/AndreyBorisovichKoval/AcousticMerge/releases/latest)

# 🎧 AcousticMerge — Offline WAV Merger (Companion Tool for AcousticLog)

**AcousticMerge** — это офлайн-утилита для **склейки коротких WAV-файлов**
(например, 200 мс сегментов, записанных AcousticLog) в один общий WAV файл.  
Поддерживает **усиление (gain %)**, **пик-нормализацию**, **кроссфейды** и цветной CLI-интерфейс.

> 🧩 AcousticMerge является **дополнительным инструментом** к основному проекту [AcousticLog](https://github.com/AndreyBorisovichKoval/AcousticLog).  
> Его использование не является обязательным, но значительно упрощает объединение коротких WAV-фрагментов,  
> если необходимо подготовить единый файл для анализа, демонстрации или отчёта.

---

## ⚙️ Назначение

AcousticMerge автоматически:
- 📂 Рекурсивно собирает все `.wav` в папке `Raw`
- 🎛️ Проверяет совместимость формата (PCM16, 1 канал, sample rate)
- 🔄 Сшивает файлы в один итоговый WAV
- 🌈 Показывает два прогресс-бара — первый для сканирования, второй для склейки
- 💡 Поддерживает цветной и эмодзи-вывод, совместим с AcousticLog папками

---

## 🧱 Требования

| Компонент | Минимум | Примечание |
|------------|----------|-------------|
| **OS** | Windows 10 / 11 | Рекомендуется NTFS-диск |
| **Go** | 1.22 или выше | Для сборки из исходников |
| **Audio** | PCM16 mono/stereo | WAV-файлы одинакового формата |

---

## 📂 Структура проекта

```text
AcousticMerge/
 ├─ cmd/
 │   └─ AcousticMerge/
 │       └─ main.go              # Точка входа CLI-утилиты
 ├─ internal/
 │   ├─ app/
 │   │   └─ app.go               # Основная логика склейки (2 прохода)
 │   └─ ui/
 │       └─ ui.go                # Цветной HELP, баннеры, прогресс-бары, логика /merge
 └─ go.mod
```

---

## 🖥️ Пример запуска

```powershell
# Показать справку
AcousticMerge.exe

# Быстрая склейка по умолчанию
AcousticMerge.exe /merge

# Усилить громкость ×1.5
AcousticMerge.exe /merge --gain-pct 150

# Склейка с нормализацией и кроссфейдом
AcousticMerge.exe --normalize -1.0 --crossfade-ms 10 /merge
```

---

## 📘 Основные параметры

| Флаг | Описание |
|------|-----------|
| `--src <путь>` | Папка с WAV-файлами (по умолчанию `DataSound_Temp\AcousticMerge\Raw`) |
| `--out <путь>` | Путь к итоговому файлу (`Result\merged.wav`, создаёт `_1.wav`, если занято) |
| `--gain-pct <число>` | Усиление громкости в процентах (100 = как есть, 150 = ×1.5) |
| `--normalize <дБ>` | Пик-нормализация до заданного уровня (напр. `-1.0`) |
| `--order name|mtime` | Сортировка по имени или времени изменения |
| `--crossfade-ms <мс>` | Кроссфейд на стыках (0 = выключено) |
| `--dry-run` | Проверка без записи итогового файла |
| `--bar-width <N>` | Ширина прогресс-бара (по умолчанию 80) |
| `--no-color`, `--no-emoji` | Отключить цвет/эмодзи в консоли |
| `--merge`, `/merge` | Быстрый режим (авто пути и запуск) |
| `--help`, `/?, -?` | Показать справку и примеры |

---

## 📁 Пути по умолчанию

| Условие | Исходники | Результат |
|----------|------------|------------|
| Если найден диск **D:** | `D:\DataSound_Temp\AcousticMerge\Raw` | `D:\DataSound_Temp\AcousticMerge\Result\merged.wav` |
| Если D: отсутствует | `C:\DataSound_Temp\AcousticMerge\Raw` | `C:\DataSound_Temp\AcousticMerge\Result\merged.wav` |

---

## 🧮 Логика работы (два прохода)

1. **PASS 1 (scan):**  
   🔍 Проверяет все файлы, считает пики, формат, кроссфейды.  
   Отображается зелёный бар прогресса.

2. **PASS 2 (merge):**  
   🔊 Сшивает данные PCM16 в один поток, с нормализацией и fade.  
   Выводится второй прогресс-бар (также 80 символов).

---

## 🧾 Примеры вывода

```
──────────────────────────
▶ AcousticMerge v0.3.0
──────────────────────────
ℹ️ starting…
   Source: D:\DataSound_Temp\AcousticMerge\Raw
   Output: D:\DataSound_Temp\AcousticMerge\Result\merged.wav
   Gain:   100.0%
   Order:  name
Format: 16000 Hz, 1 ch, 16 bps (PCM16)
🟩 PASS1 scan:  [████████████████████████████████████████] 100.00% (2541/2541)
🟩 PASS2 merge: [████████████████████████████████████████] 100.00% (2541/2541)
✅ Output saved: D:\DataSound_Temp\AcousticMerge\Result\merged.wav
```

---

## 🧠 Советы

- Сохраняйте папку `Raw` структурированной (по датам или часам) — так легче контролировать объём.
- Для многих тысяч файлов рекомендуется режим `--order name` и диск SSD.
- Если видите 🛑 или ⚠️ — проверьте формат (WAV 16-бит, 1 канал, одинаковая частота).

---

## 🧾 История версий

| Версия | Дата | Изменения |
|---------|------|------------|
| **v0.3.0** | 2025-10-28 | Разделение по файлам (cmd/internal), цветной HELP, авто путь D:→C:, двойной прогресс-бар |
| **v0.2.x** | 2025-10-26 | Добавлена структура и алиасы `/merge`, `--gain-pct` → `--gain` |
| **v0.1.x** | 2025-10-25 | Базовая реализация в одном файле (main.go) |

---

## 👤 Автор

**Andrey Koval**  
📍 Dushanbe, Tajikistan  
📧 [andrey.koval.dev@gmail.com](mailto:andrey.koval.dev@gmail.com)  
📦 [github.com/AndreyBorisovichKoval/AcousticMerge](https://github.com/AndreyBorisovichKoval/AcousticMerge)

---

## ⚖️ Лицензия

Проект распространяется по лицензии **MIT**.  
Свободно используйте, модифицируйте и распространяйте с указанием автора.

---

> 🎵 **AcousticMerge** — компаньон для **AcousticLog**, созданный для удобной и наглядной склейки аудио в один WAV-файл.
