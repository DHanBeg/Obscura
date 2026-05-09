#!/bin/sh
# Clone curated external skill repos to .claude/skills/external/
# These are NOT committed (too big); each dev runs this once.

set -e
ROOT="$(git rev-parse --show-toplevel)"
DST="$ROOT/.claude/skills/external"
mkdir -p "$DST"
cd "$DST"

repos="
  anthropics/skills:anthropics
  obra/superpowers:obra-superpowers
  vercel-labs/agent-skills:vercel-agent-skills
  vercel-labs/next-skills:vercel-next-skills
  vercel-labs/skills:vercel-skills
  vercel-labs/agent-browser:vercel-agent-browser
  expo/skills:expo-skills
  wshobson/agents:wshobson-agents
  mattpocock/skills:mattpocock-skills
  firebase/agent-skills:firebase-agent-skills
  neondatabase/agent-skills:neon-agent-skills
  microsoft/azure-skills:microsoft-azure-skills
  currents-dev/playwright-best-practices-skill:currents-playwright
  remotion-dev/skills:remotion-skills
  xixu-me/skills:xixu-skills
  coreyhaines31/marketingskills:coreyhaines-marketing
  supabase-community/skills:supabase-skills
  cloudflare/agents-starter:cloudflare-agents
"

for repo in $repos; do
    src="${repo%%:*}"
    dst="${repo##*:}"
    if [ -d "$dst" ]; then
        echo "→ $dst already exists, pulling..."
        (cd "$dst" && git pull --ff-only 2>/dev/null) || echo "  (pull failed; keep existing)"
    else
        echo "→ Cloning $src..."
        git clone --depth 1 "https://github.com/$src.git" "$dst" 2>&1 | tail -1 || echo "  ✗ failed"
    fi
done

echo ""
echo "✓ External skills installed to: $DST"
echo "  Total skill repos: $(ls -d */ 2>/dev/null | wc -l)"
echo "  Total SKILL.md files: $(find . -name SKILL.md 2>/dev/null | wc -l)"
echo "  Total markdown files: $(find . -name '*.md' 2>/dev/null | wc -l)"
