#!/bin/zsh
set -e

PROJECT_DIR="${0:A:h}"
ZSHRC="$HOME/.zshrc"
MARKER="# lk bookmark manager"
SOURCE_LINE="source \"$PROJECT_DIR/lk.zsh\""

if ! command -v uv >/dev/null 2>&1; then
  echo "uv is not installed. Install it from https://github.com/astral-sh/uv and re-run."
  exit 1
fi

if grep -Fq "$MARKER" "$ZSHRC" 2>/dev/null; then
  echo "lk alias already present in $ZSHRC. Nothing to do."
else
  {
    echo ""
    echo "$MARKER"
    echo "$SOURCE_LINE"
  } >> "$ZSHRC"
  echo "Added lk alias to $ZSHRC."
fi

echo "Priming uv environment..."
uv sync --project "$PROJECT_DIR"

echo ""
echo "Done. Open a new terminal (or run 'source ~/.zshrc') and try: lk"
