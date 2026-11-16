# HostPulse

**Un monitor del sistema para la terminal, moderno y sin dependencias.** Algo como `htop`, en un único binario:
escribe `hostpulse` y verás en vivo CPU (total y por núcleo), memoria, swap, red, E/S de disco, discos y procesos.

- Barras con degradado de color, minigráficas de historial (CPU, red ↓/↑) y paneles con bordes redondeados.
- Tabla de procesos con CPU instantánea (100 = un núcleo, igual que htop), memoria, hilos y línea de comandos completa.
- Se adapta al tamaño de la terminal (en pantallas anchas pone CPU y memoria/red lado a lado).
- Solo **lee** `/proc` y `/sys`. No abre puertos ni hace conexiones de red. Solo Linux.

## Uso

```bash
hostpulse            # abre el monitor
hostpulse -d 2       # actualiza cada 2 segundos (por defecto 1)
hostpulse --no-color # sin colores (también respeta NO_COLOR)
hostpulse -v         # versión
```

| Tecla | Acción |
|---|---|
| `↑` `↓` / `j` `k`, `PgUp` `PgDn`, `Inicio` `Fin` | mover la selección |
| `c` · `m` · `p` · `n` | ordenar por CPU · memoria · PID · nombre (repetir la tecla invierte el orden) |
| `r` | invertir el orden |
| `/` | filtrar por comando, usuario o PID (`Enter` acepta, `Esc` borra) |
| `espacio` | pausar / reanudar |
| `q` / `Ctrl-C` | salir |

## Instalación

```bash
bash scripts/build.sh          # genera dist/hostpulse-linux-amd64 y -arm64 (necesita Go 1.24+)
sudo bash scripts/install.sh   # copia el binario a /usr/local/bin/hostpulse
```

O compílalo directamente en la máquina: `go build -o hostpulse ./cmd/hostpulse`.

## Estructura

`cmd/hostpulse` (CLI) · `internal/tui` (interfaz: dibujo, teclas, paneles) · `internal/sysinfo` (lectura de `/proc`).
