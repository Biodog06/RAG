#!/usr/bin/env python3
"""
fetch-issue.py - Fetch a GitHub issue and save as markdown

Usage:
    ./fetch-issue.py <issue-id> [repo] [--preview] [--start-pipeline]

Arguments:
    issue-id        Required. GitHub issue number (positive integer)
    repo            Optional. Repository in owner/repo format (auto-detects if omitted)
    --preview       Optional. Show output without writing to file
    --start-pipeline Optional. Start orchestrator after fetching
"""

import sys
import json
import subprocess
import os
import re
from datetime import datetime
from pathlib import Path


# ANSI color codes
class Colors:
    RED = '\033[0;31m'
    GREEN = '\033[0;32m'
    YELLOW = '\033[1;33m'
    BLUE = '\033[0;34m'
    NC = '\033[0m'  # No Color


def print_error(message):
    """Print error message in red"""
    print(f"{Colors.RED}❌ Error: {message}{Colors.NC}", file=sys.stderr)


def print_success(message):
    """Print success message in green"""
    print(f"{Colors.GREEN}{message}{Colors.NC}")


def print_info(message):
    """Print info message in blue"""
    print(f"{Colors.BLUE}{message}{Colors.NC}")


def print_warning(message):
    """Print warning message in yellow"""
    print(f"{Colors.YELLOW}{message}{Colors.NC}")


def parse_arguments():
    """Parse command line arguments"""
    args = {
        'issue_id': None,
        'repo': None,
        'preview': False,
        'start_pipeline': False
    }
    
    positional = []
    i = 1
    while i < len(sys.argv):
        arg = sys.argv[i]
        if arg == '--preview':
            args['preview'] = True
        elif arg == '--start-pipeline':
            args['start_pipeline'] = True
        else:
            positional.append(arg)
        i += 1
    
    if len(positional) > 0:
        args['issue_id'] = positional[0]
    if len(positional) > 1:
        args['repo'] = positional[1]
    
    return args


def validate_issue_id(issue_id):
    """Validate issue ID format"""
    if not issue_id:
        print_error("ISSUE_ID is required")
        print("Usage: ./fetch-issue.py <issue-id> [repo] [--preview] [--start-pipeline]")
        sys.exit(1)
    
    if not re.match(r'^\d+$', issue_id) or int(issue_id) < 1:
        print_error("ISSUE_ID must be a positive integer")
        print(f"Provided: {issue_id}")
        sys.exit(1)


def validate_repo_format(repo):
    """Validate repository format"""
    if repo and not re.match(r'^[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+$', repo):
        print_error("REPO must be in 'owner/repo' format")
        print(f"Provided: {repo}")
        print("Example: microsoft/vscode")
        sys.exit(1)


def check_gh_cli():
    """Check if GitHub CLI is installed"""
    try:
        subprocess.run(['gh', '--version'], capture_output=True, check=True)
    except (subprocess.CalledProcessError, FileNotFoundError):
        print_error("GitHub CLI (gh) is not installed")
        print("")
        print("Install GitHub CLI:")
        print("  macOS:   brew install gh")
        print("  Windows: winget install --id GitHub.cli")
        print("  Linux:   https://github.com/cli/cli/blob/trunk/docs/install_linux.md")
        sys.exit(1)


def check_gh_auth():
    """Check if GitHub CLI is authenticated"""
    try:
        subprocess.run(['gh', 'auth', 'status'], capture_output=True, check=True)
    except subprocess.CalledProcessError:
        print_error("GitHub CLI is not authenticated")
        print("")
        print("Authenticate with GitHub:")
        print("  gh auth login")
        sys.exit(1)


def detect_repository():
    """Auto-detect repository from current directory"""
    try:
        result = subprocess.run(
            ['gh', 'repo', 'view', '--json', 'nameWithOwner', '-q', '.nameWithOwner'],
            capture_output=True,
            text=True,
            check=True
        )
        repo = result.stdout.strip()
        print_info(f"📍 Detected repository: {repo}")
        return repo
    except subprocess.CalledProcessError:
        print_error("Could not detect repository")
        print("")
        print("Either:")
        print("  1. Run this command from within a git repository, or")
        print("  2. Specify the repository: ./fetch-issue.py <issue-id> owner/repo")
        sys.exit(1)


def fetch_issue(issue_id, repo):
    """Fetch issue from GitHub"""
    print_info(f"🔄 Fetching issue #{issue_id} from {repo}...")
    
    try:
        result = subprocess.run(
            ['gh', 'issue', 'view', issue_id, '--repo', repo, '--json',
             'title,body,labels,assignees,milestone,createdAt,number,url,state'],
            capture_output=True,
            text=True,
            check=True
        )
        return json.loads(result.stdout)
    except subprocess.CalledProcessError as e:
        error_output = e.stderr
        if 'Could not resolve' in error_output or 'not found' in error_output:
            print_error(f"Issue #{issue_id} not found in {repo}")
            print("")
            print("Verify:")
            print("  1. The issue number is correct")
            print("  2. You have access to the repository")
            print("  3. The repository name is correct")
        elif 'network' in error_output or 'connection' in error_output:
            print_error("Network error while fetching issue")
            print("")
            print("Check your internet connection and try again")
        else:
            print_error("Failed to fetch issue")
            print(error_output)
        sys.exit(1)


def format_issue(issue_data, repo):
    """Format issue data as markdown"""
    title = issue_data['title']
    body = issue_data.get('body') or "No description provided."
    number = issue_data['number']
    url = issue_data['url']
    state = issue_data['state']
    created_at = issue_data['createdAt'].split('T')[0]
    
    labels = ', '.join([label['name'] for label in issue_data.get('labels', [])]) or "None"
    assignees = ', '.join([assignee['login'] for assignee in issue_data.get('assignees', [])]) or "Unassigned"
    milestone_data = issue_data.get('milestone')
    milestone = milestone_data.get('title') if milestone_data else "None"
    
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    
    output = f"""# GitHub Issue #{number}: {title}

**Repository:** {repo}
**URL:** {url}
**State:** {state}
**Labels:** {labels}
**Created:** {created_at}

---

## Description

{body}

---

## Metadata

- **Assignees:** {assignees}
- **Milestone:** {milestone}

---

_Fetched on {timestamp} using fetch-github-issue skill_"""
    
    return output


def main():
    """Main execution function"""
    args = parse_arguments()
    
    # Validate inputs
    validate_issue_id(args['issue_id'])
    validate_repo_format(args['repo'])
    
    # Check prerequisites
    check_gh_cli()
    check_gh_auth()
    
    # Detect repository if not provided
    repo = args['repo'] if args['repo'] else detect_repository()
    
    # Fetch issue
    issue_data = fetch_issue(args['issue_id'], repo)
    
    # Format output
    output = format_issue(issue_data, repo)
    
    # Preview mode
    if args['preview']:
        print_success(f"📋 Preview of Issue #{args['issue_id']}:")
        print("")
        print(output)
        print("")
        print_warning("ℹ️  Preview mode - no file written")
        print("Remove --preview to save the file")
        return
    
    # Write output file
    output_dir = Path(".tot-docs/requirements/issues")
    output_file = output_dir / f"{args['issue_id']}.md"
    
    output_dir.mkdir(parents=True, exist_ok=True)
    
    if output_file.exists():
        print_warning(f"⚠️  Warning: File already exists at {output_file}")
        print("Overwriting with fresh content...")
    
    output_file.write_text(output)
    
    print_success("✅ Issue fetched successfully!")
    print("")
    print(f"Output: {output_file}")
    
    # Pipeline integration
    if args['start_pipeline']:
        print("")
        print_info("🚀 Starting orchestrator pipeline...")
        print("")
        print("To start the pipeline manually, run:")
        print(f"  /orchestrator INPUT='{output_file}' MODE=feature")


if __name__ == '__main__':
    main()
