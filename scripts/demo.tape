# Demo GIF do Peapod. Gerar com:  vhs scripts/demo.tape
# (requer VHS — brew install vhs — e OrbStack/Docker rodando)
Output docs/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1280
Set Height 540
Set Theme "Dracula"
Set TypingSpeed 28ms
Set Margin 20
Set MarginFill "#1d9e75"
Set BorderRadius 8
Set Padding 18

Hide
Type "export PATH=/opt/homebrew/bin:$PATH PS1='$ '; clear" Enter
Show

Type "# sandbox Python isolado, sem rede e auditado" Enter
Sleep 800ms
Type "ID=$(peapod sandbox create python:3.12-slim --net none)" Enter
Sleep 2500ms
Type "peapod sandbox exec $ID python3 -c 'print(6*7)'" Enter
Sleep 1800ms
Type 'peapod sandbox exec $ID sh -lc "wget -T2 -qO- http://example.com >/dev/null 2>&1 && echo tem-rede || echo sem-rede-isolado"' Enter
Sleep 2200ms
Type "peapod sandbox history $ID   # trilha de auditoria" Enter
Sleep 2200ms
Type "peapod sandbox rm $ID" Enter
Sleep 1800ms
