from datetime import date, timedelta

import pandas as pd

# -----------------------------
# CONFIGURATION
# -----------------------------
start_date = date(2026, 2, 8)   # PP05 start
end_date   = date(2026, 5, 2)   # PP10 end

# Teams and auto-generated members
teams = {
    "Team A": [f"A{i:02d}" for i in range(1, 7)],
    "Team B": [f"B{i:02d}" for i in range(1, 7)],
    "Team C": [f"C{i:02d}" for i in range(1, 7)],
    "Team D": [f"D{i:02d}" for i in range(1, 7)],
    "Team E": [f"E{i:02d}" for i in range(1, 7)],
    "Team F": [f"F{i:02d}" for i in range(1, 6)],
}

# COA2 cycle: 3 ON, 2 OFF, 2 ON, 3 OFF
coa2_cycle = ["Work", "Work", "Work", "Off", "Off", "Work", "Work", "Off", "Off", "Off"]

shifts = ["Day 0700-1900", "Night 1900-0700"]

# -----------------------------
# BUILD DAILY SCHEDULE
# -----------------------------
rows = []
current_date = start_date
day_index = 0

while current_date <= end_date:
    week_num = current_date.isocalendar()[1]
    pay_period = f"PP{5 + ((current_date - start_date).days // 14):02d}"

    for team_index, (team, members) in enumerate(teams.items()):
        # Stagger teams by 2 days to rotate coverage
        cycle_day = (day_index + team_index * 2) % len(coa2_cycle)
        status = coa2_cycle[cycle_day]

        for person_index, name in enumerate(members):
            assignment = ""
            hours = 0

            if status == "Work":
                assignment = shifts[(day_index + person_index) % 2]
                hours = 12

            rows.append([
                current_date,
                pay_period,
                week_num,
                team,
                name,
                status,
                assignment,
                hours,
            ])

    current_date += timedelta(days=1)
    day_index += 1

daily_df = pd.DataFrame(rows, columns=[
    "Date", "Pay Period", "Week", "Team", "Name", "Status", "Shift", "Paid Hours",
])

# -----------------------------
# WEEKLY SUBTOTALS
# -----------------------------
weekly_df = (
    daily_df
    .groupby(["Name", "Week"], as_index=False)["Paid Hours"]
    .sum()
    .rename(columns={"Paid Hours": "Weekly Hours"})
)

# -----------------------------
# PAY PERIOD SUBTOTALS
# -----------------------------
pp_summary = (
    daily_df
    .groupby(["Name", "Pay Period"], as_index=False)["Paid Hours"]
    .sum()
    .rename(columns={"Paid Hours": "Total Hours"})
)

# -----------------------------
# EXPORT TO EXCEL
# -----------------------------
output_file = "COA2_2026_Individual_Schedule_PP05_PP10.xlsx"

with pd.ExcelWriter(output_file, engine="xlsxwriter") as writer:
    daily_df.to_excel(writer, sheet_name="Daily_Assignments", index=False)
    weekly_df.to_excel(writer, sheet_name="Weekly_Summary", index=False)
    pp_summary.to_excel(writer, sheet_name="PayPeriod_Summary", index=False)
    daily_df.to_excel(writer, sheet_name="Pivot_Source", index=False)

print(f"Excel schedule generated: {output_file}")
