#!/usr/bin/env python3
"""
Shift Schedule Generator
24/7 Operations — Two Teams, Three 8-Hour Shifts
80 hours per employee per 2-week pay period
"""

import csv
import os
from datetime import date, timedelta

# ── Configuration ──────────────────────────────────────────────────────────

START_DATE = date(2026, 1, 26)  # Monday
PAY_PERIOD_DAYS = 14

SHIFTS = {
    "D": "Day     (0600–1400)",
    "S": "Swing   (1400–2200)",
    "N": "Night   (2200–0600)",
}

HOURS_PER_SHIFT = 8
TARGET_HOURS = 80  # per 2-week pay period
SHIFTS_PER_PERIOD = TARGET_HOURS // HOURS_PER_SHIFT  # 10 shifts

# Team 1: 16 people → 6 Day / 5 Swing / 5 Night
# Team 2: 15 people → 5 Day / 5 Swing / 5 Night
TEAMS = {
    "Team 1": {
        "size": 16,
        "split": {"D": 6, "S": 5, "N": 5},
    },
    "Team 2": {
        "size": 15,
        "split": {"D": 5, "S": 5, "N": 5},
    },
}


# ── Days-off patterns ─────────────────────────────────────────────────────
# Index: 0=Mon … 6=Sun
# Each person gets 2 consecutive days off per week.
# Patterns are designed to spread coverage as evenly as possible.

DAYS_OFF = {
    5: [
        (2, 3),  # Wed–Thu
        (5, 6),  # Sat–Sun
        (0, 1),  # Mon–Tue
        (3, 4),  # Thu–Fri
        (6, 0),  # Sun–Mon
    ],
    6: [
        (0, 1),  # Mon–Tue
        (2, 3),  # Wed–Thu
        (4, 5),  # Fri–Sat
        (6, 0),  # Sun–Mon
        (1, 2),  # Tue–Wed
        (3, 4),  # Thu–Fri
    ],
}

DAY_NAMES = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]


# ── Schedule builder ──────────────────────────────────────────────────────

def build_team(team_name, config):
    """Return list of employee dicts with 14-day assignment arrays."""
    employees = []
    emp_num = 1
    prefix = "T1" if "1" in team_name else "T2"

    for shift_code, count in config["split"].items():
        pattern = DAYS_OFF[count]
        for i in range(count):
            off_days = set(pattern[i])
            assignments = []
            for day_idx in range(PAY_PERIOD_DAYS):
                weekday = day_idx % 7  # 0=Mon since start is Monday
                assignments.append("OFF" if weekday in off_days else shift_code)

            employees.append({
                "id": f"{prefix}-{emp_num:02d}",
                "shift": shift_code,
                "off_pattern": sorted(off_days),
                "assignments": assignments,
                "hours": sum(1 for a in assignments if a != "OFF") * HOURS_PER_SHIFT,
            })
            emp_num += 1

    return employees


def coverage_grid(employees):
    """Return per-day, per-shift headcount for 14 days."""
    grid = []
    for day_idx in range(PAY_PERIOD_DAYS):
        counts = {"D": 0, "S": 0, "N": 0}
        for emp in employees:
            a = emp["assignments"][day_idx]
            if a != "OFF":
                counts[a] += 1
        grid.append(counts)
    return grid


# ── Output: Markdown ──────────────────────────────────────────────────────

def date_headers():
    """Return list of formatted date strings for 14 days."""
    return [(START_DATE + timedelta(days=i)).strftime("%a %m/%d") for i in range(PAY_PERIOD_DAYS)]


def write_markdown(path, teams_data):
    headers = date_headers()

    with open(path, "w") as f:
        f.write("# 24/7 Shift Schedule\n\n")
        f.write(f"**Pay period:** {START_DATE.strftime('%B %d, %Y')} – "
                f"{(START_DATE + timedelta(days=13)).strftime('%B %d, %Y')}\n\n")

        f.write("## Parameters\n\n")
        f.write("| Parameter | Value |\n")
        f.write("|---|---|\n")
        f.write(f"| Shifts | Day (0600–1400), Swing (1400–2200), Night (2200–0600) |\n")
        f.write(f"| Shift length | {HOURS_PER_SHIFT} hours |\n")
        f.write(f"| Pay period | 2 weeks |\n")
        f.write(f"| Target hours | {TARGET_HOURS} per pay period ({SHIFTS_PER_PERIOD} shifts) |\n")
        f.write(f"| Days off | 2 consecutive days per week (staggered for coverage) |\n")
        f.write("\n")

        for team_name, (employees, grid) in teams_data.items():
            cfg = TEAMS[team_name]
            split_str = " / ".join(f"{v} {k}" for k, v in cfg["split"].items())
            f.write(f"---\n\n## {team_name} — {cfg['size']} employees ({split_str})\n\n")

            # Schedule grid
            f.write("### Schedule\n\n")
            f.write("| Employee | Shift |")
            # Week 1
            f.write(" " + " | ".join(headers[:7]) + " |")
            # Week 2
            f.write(" " + " | ".join(headers[7:]) + " |\n")

            f.write("|---|---|")
            f.write("---|" * 14 + "\n")

            current_shift = None
            for emp in employees:
                if emp["shift"] != current_shift:
                    current_shift = emp["shift"]
                    shift_label = {"D": "Day", "S": "Swing", "N": "Night"}[current_shift]
                    f.write(f"| **{shift_label}** | | |" + " |" * 13 + "\n")

                row = f"| {emp['id']} | {emp['shift']} |"
                for a in emp["assignments"]:
                    if a == "OFF":
                        row += " OFF |"
                    else:
                        row += f" **{a}** |"
                f.write(row + "\n")

            # Hours verification
            f.write(f"\n*All employees: {employees[0]['hours']} hours/pay period*\n\n")

            # Coverage analysis
            f.write("### Daily Coverage (headcount per shift)\n\n")
            f.write("| Shift |")
            f.write(" " + " | ".join(headers) + " |\n")
            f.write("|---|" + "---|" * 14 + "\n")

            for shift_code in ["D", "S", "N"]:
                label = {"D": "Day", "S": "Swing", "N": "Night"}[shift_code]
                row = f"| {label} |"
                for day_idx in range(PAY_PERIOD_DAYS):
                    count = grid[day_idx][shift_code]
                    row += f" {count} |"
                f.write(row + "\n")

            # Total on duty
            row = "| **Total** |"
            for day_idx in range(PAY_PERIOD_DAYS):
                total = sum(grid[day_idx].values())
                row += f" **{total}** |"
            f.write(row + "\n\n")

        # Combined coverage
        f.write("---\n\n## Combined Coverage (Both Teams)\n\n")
        f.write("| Shift |")
        f.write(" " + " | ".join(headers) + " |\n")
        f.write("|---|" + "---|" * 14 + "\n")

        all_grids = list(teams_data.values())
        for shift_code in ["D", "S", "N"]:
            label = {"D": "Day", "S": "Swing", "N": "Night"}[shift_code]
            row = f"| {label} |"
            for day_idx in range(PAY_PERIOD_DAYS):
                total = sum(g[day_idx][shift_code] for _, g in all_grids)
                row += f" {total} |"
            f.write(row + "\n")

        row = "| **Total** |"
        for day_idx in range(PAY_PERIOD_DAYS):
            total = sum(
                sum(g[day_idx].values()) for _, g in all_grids
            )
            row += f" **{total}** |"
        f.write(row + "\n\n")

        # Footnotes
        f.write("---\n\n## Notes\n\n")
        f.write("- **D** = Day shift (0600–1400)\n")
        f.write("- **S** = Swing shift (1400–2200)\n")
        f.write("- **N** = Night shift (2200–0600)\n")
        f.write("- Each employee works 5 days on / 2 days off per week (consecutive days off)\n")
        f.write("- Days off are staggered across the week to maintain 24/7 coverage\n")
        f.write("- This schedule assigns employees to fixed shifts; "
                "to rotate shifts (e.g., monthly), swap shift assignments between groups\n")


# ── Output: CSV ───────────────────────────────────────────────────────────

def write_csv(path, team_name, employees):
    headers = date_headers()
    with open(path, "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["Employee", "Shift"] + headers + ["Total Hours"])
        for emp in employees:
            w.writerow([emp["id"], emp["shift"]] + emp["assignments"] + [emp["hours"]])


# ── Main ──────────────────────────────────────────────────────────────────

def main():
    os.makedirs("output", exist_ok=True)

    teams_data = {}
    for team_name, config in TEAMS.items():
        employees = build_team(team_name, config)
        grid = coverage_grid(employees)
        teams_data[team_name] = (employees, grid)

        csv_name = team_name.lower().replace(" ", "_")
        write_csv(f"output/{csv_name}.csv", team_name, employees)
        print(f"  CSV → output/{csv_name}.csv ({len(employees)} employees)")

    write_markdown("output/schedule.md", teams_data)
    print(f"  Markdown → output/schedule.md")

    # Print summary
    print()
    for team_name, (employees, grid) in teams_data.items():
        cfg = TEAMS[team_name]
        print(f"{team_name} ({cfg['size']} employees):")
        split_str = ", ".join(f"{v} on {k}" for k, v in cfg["split"].items())
        print(f"  Shift split: {split_str}")
        min_on = min(sum(g.values()) for g in grid)
        max_on = max(sum(g.values()) for g in grid)
        print(f"  Daily total on duty: {min_on}–{max_on}")
        for s in ["D", "S", "N"]:
            lo = min(g[s] for g in grid)
            hi = max(g[s] for g in grid)
            print(f"    {s}: {lo}–{hi} per day")
        print(f"  Hours/employee: {employees[0]['hours']}")
        print()


if __name__ == "__main__":
    main()
