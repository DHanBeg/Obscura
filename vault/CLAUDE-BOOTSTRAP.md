# Obsidian Vault Guidelines - Bootstrap Template

**Getting Started with Claude Code + Obsidian**

## Quick Setup

1. **Start every session**: Run `git pull` to sync latest changes
2. **After changes**: Commit and push to preserve your work
3. **Use built-in tools**: Prefer WebSearch and WebFetch for web content

## Version Control Best Practices

**CRITICAL - START EVERY SESSION**: Always run `git pull` at the beginning of
each new Claude session to ensure you have the latest changes from the remote
repository.

**Commit workflow**:

- After creating new notes: `git add .` → `git commit -m "message"` → `git push`
- After significant edits: Commit and push immediately
- Use `git status` to check for modifications
- When agents modify files: Always commit those changes

## Folder Structure (PARA Method)

```
vault/
├── 00_Inbox/           # Temporary capture point
├── 01_Projects/        # Time-bound initiatives
├── 02_Areas/           # Ongoing responsibilities
├── 03_Resources/       # Reference materials
├── 04_Archive/         # Completed/inactive items
├── 05_Attachments/     # Images, PDFs, etc.
│   └── Organized/      # Processed attachments
└── 06_Metadata/        # Documentation & templates
    ├── Reference/      # Guides and standards
    ├── Plans/          # Strategic documents
    └── Templates/      # Reusable structures
```

## PARA Method Details

### Projects (01)

- Time-bound initiatives with clear completion criteria
- Examples: Writing a paper, developing a presentation
- Recommended subfolders: Research/, Drafts/, References/, Output/

### Areas (02)

- Ongoing responsibilities without end dates
- Examples: Health, Finances, Professional Development
- Create dedicated notes with links to related resources

### Resources (03)

- Topics of interest for reference
- Knowledge bases organized by subject
- Use for information not tied to specific projects

### Archive (04)

- Completed or inactive items
- Maintain same folder structure as active sections
- Review periodically for reactivation

## Inbox Management

### Core Principles

- Inbox is temporary, not permanent storage
- Process weekly using Capture → Process → Organize workflow
- Maintain <20 items at any time

### Files to Keep in Inbox

- **CRITICAL**: Files with number prefixes (00-06) stay permanently
- Recent daily/weekly summaries (last 3 months)
- Active notes being processed

### Processing Workflow

1. Delete obsolete information
2. Move relevant material to PARA locations
3. Convert actions into project tasks
4. Tag items needing more processing with `#needs-processing`

## File Organization Guidelines

### Naming Conventions

- Daily notes: `YYYY-MM-DD - Topic`
- Meeting notes: `Meeting - [Topic] - YYYY-MM-DD`
- Ideas: `Idea - [Brief Description]`
- Resources: `Resource - [Topic] - [Source]`

### Movement Rules

- Use `mv` command (not `cp`) to avoid duplicates
- Verify destination folders exist first
- Update internal links after moves
- Add YAML frontmatter when organizing

## Attachments Management

### Organization

- Store all non-text files in `05_Attachments/`
- Processed files → `05_Attachments/Organized/`
- Naming: `[RelatedNote]_[Description].[ext]`

### Helper Scripts

```bash
pnpm attachments:list        # List unprocessed files
pnpm attachments:organized   # Count organized files
pnpm attachments:orphans     # Find unreferenced files
pnpm attachments:update-links # Update links after moving
```

## Web Content Workflow

### Built-in Tools (Preferred)

- **WebSearch**: For general web searches
- **WebFetch**: For specific URLs
- Save to appropriate folder based on content type

### Custom Scripts (When Needed)

- Single URL: `pnpm firecrawl:scrape <url> <output>`
- Batch URLs: `pnpm firecrawl:batch <url1> <url2>`
- Saves to `00_Inbox/Clippings/` with frontmatter

## Writing Style Guidelines

### Structure

- Use `[[WikiLinks]]` for internal references
- Include YAML frontmatter (dates, tags, status)
- Consistent Markdown formatting
- Specific, consistent tags

### Style Preferences

- Direct and confident statements
- Avoid clichéd transitions
- Let statements stand on their own
- No unnecessary lead-ins

## AI Assistant Guidelines

### Before Any Organization

1. Map complete folder structure: `find . -type d | sort`
2. Document in `06_Metadata/STRUCTURE.md`
3. Verify all destination folders exist

### Working with Content

- Respect numbered core files (never move 00-06 prefixed files)
- Always use `mv` not `cp` when organizing
- Preserve and update bidirectional links
- Add appropriate YAML frontmatter

### Simple Commands Only

- **REQUIRED**: Direct, basic commands without filtering
- **FORBIDDEN**: Complex regex, piped commands, find with filters
- Example RIGHT: `ls -1` then manually select files
- Example WRONG: `ls | grep pattern` or `find . -name "*.png"`

## Daily Workflows

### Start of Day

1. Run `git pull`
2. Check inbox for items to process
3. Review active projects

### End of Day

1. Process new inbox items
2. Commit and push changes
3. Update project notes

### Weekly Review

1. Process entire inbox
2. Archive completed projects
3. Update area notes
4. Review and consolidate resources

## Project Lifecycle

### Starting a Project

1. Create folder in `01_Projects/[ProjectName]`
2. Add subfolders: Research/, Drafts/, Output/
3. Create README with objectives and timeline

### During Project

- Keep all related materials in project folder
- Link to relevant resources and areas
- Regular commits to track progress

### Completing a Project

1. Create project summary note
2. Move entire folder to `04_Archive/`
3. Update relevant area notes
4. Commit with completion message

## Best Practices

### Organization

- Keep folder structure shallow (max 3 levels)
- Create subfolders only with 7+ related notes
- Use linking over deep nesting
- Include README in major folders

### Content Creation

- Capture first, organize later
- One idea per note
- Link generously
- Tag consistently

### Maintenance

- Weekly inbox processing
- Monthly project reviews
- Quarterly archive cleanup
- Regular git commits

---

_This is a bootstrap template. Customize based on your workflow and needs._
