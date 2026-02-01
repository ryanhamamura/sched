from datetime import date, timedelta

import pandas as pd

# ── Configuration ──────────────────────────────────────────────────────────

START_DATE = date(2026, 2, 8)   # PP05 start
END_DATE   = date(2026, 5, 2)   # PP10 end

STAFFING_MODE = "3x8"           # "3x8" (preferred) or "2x12" (fallback)
PAY_PERIOD_DAYS = 14
MAX_HOURS_PER_PAY_PERIOD = 80

SHIFTS_3x8  = {"Days": "0700-1500", "Mids": "1500-2300", "Nights": "2300-0700"}
SHIFTS_2x12 = {"Days": "0700-1900", "Nights": "1900-0700"}

CREWS = {
    "Crew 1": [f"C1-{i:02d}" for i in range(1, 10)],   # 9
    "Crew 2": [f"C2-{i:02d}" for i in range(1, 10)],   # 9
    "Crew 3": [f"C3-{i:02d}" for i in range(1, 10)],   # 9
    "Crew 4": [f"C4-{i:02d}" for i in range(1, 9)],    # 8
}

# 11 on, 3 off — 88h raw, 80h cap converts 1 day to admin per PP
ON_OFF_PATTERN = [True] * 11 + [False] * 3

# Stagger crews so exactly 3 are on duty every day
CREW_OFFSETS = [0, 4, 7, 10]

SHIFT_ROTATION_3x8  = ["Days", "Mids", "Nights"]
SHIFT_ROTATION_2x12 = ["Days", "Nights"]

PAX_PER_SHIFT = 2

OUTPUT_FILE = "COA2_2026_Individual_Schedule_PP05_PP10.xlsx"


# ── Helper Functions ───────────────────────────────────────────────────────

def make_pay_periods(start: date, end: date) -> list[tuple[int, str, date, date]]:
    """Return list of (index, label, pp_start, pp_end) covering start..end."""
    periods = []
    pp_start = start
    idx = 0
    base_pp_num = 5  # first pay period is PP05
    while pp_start <= end:
        pp_end = pp_start + timedelta(days=PAY_PERIOD_DAYS - 1)
        if pp_end > end:
            pp_end = end
        label = f"PP{base_pp_num + idx:02d}"
        periods.append((idx, label, pp_start, pp_end))
        pp_start = pp_end + timedelta(days=1)
        idx += 1
    return periods


def is_on_duty(day_in_period: int, crew_idx: int) -> bool:
    pos = (day_in_period - CREW_OFFSETS[crew_idx]) % len(ON_OFF_PATTERN)
    return ON_OFF_PATTERN[pos]


def get_shift_type(crew_idx: int, pp_idx: int) -> str:
    if STAFFING_MODE == "3x8":
        types = SHIFT_ROTATION_3x8
    else:
        types = SHIFT_ROTATION_2x12
    return types[(pp_idx + crew_idx) % len(types)]


def hours_per_shift() -> int:
    return 8 if STAFFING_MODE == "3x8" else 12


def build_daily_schedule() -> pd.DataFrame:
    pay_periods = make_pay_periods(START_DATE, END_DATE)
    crew_names = list(CREWS.keys())
    h = hours_per_shift()
    shift_types = SHIFT_ROTATION_3x8 if STAFFING_MODE == "3x8" else SHIFT_ROTATION_2x12
    rows = []

    for pp_idx, pp_label, pp_start, pp_end in pay_periods:
        person_hours: dict[str, int] = {}

        day = pp_start
        day_in_period = 0
        while day <= pp_end:
            week_num = day.isocalendar()[1]

            # Determine which crews are on duty today
            on_duty_crews = [
                (idx, name) for idx, name in enumerate(crew_names)
                if is_on_duty(day_in_period, idx)
            ]

            # Assign each on-duty crew to a shift, rotating by pp_idx
            crew_shift_map: dict[int, str] = {}
            for rank, (crew_idx, _) in enumerate(on_duty_crews):
                crew_shift_map[crew_idx] = shift_types[
                    (pp_idx + rank) % len(shift_types)
                ]

            for crew_idx, crew_name in enumerate(crew_names):
                on_duty = crew_idx in crew_shift_map

                for person in CREWS[crew_name]:
                    if not on_duty:
                        status, shift, paid = "Off", "", 0
                    else:
                        accumulated = person_hours.get(person, 0)
                        if accumulated + h > MAX_HOURS_PER_PAY_PERIOD:
                            status, shift, paid = "Admin", "", 0
                        else:
                            status = "Watch"
                            shift = crew_shift_map[crew_idx]
                            paid = h
                            person_hours[person] = accumulated + h

                    rows.append([
                        day, pp_label, week_num, crew_name,
                        person, status, shift, paid,
                    ])

            day += timedelta(days=1)
            day_in_period += 1

    return pd.DataFrame(rows, columns=[
        "Date", "Pay Period", "Week", "Crew", "Name",
        "Status", "Shift", "Paid Hours",
    ])


def build_weekly_summary(daily_df: pd.DataFrame) -> pd.DataFrame:
    return (
        daily_df
        .groupby(["Name", "Crew", "Week"], as_index=False)["Paid Hours"]
        .sum()
        .rename(columns={"Paid Hours": "Weekly Hours"})
    )


def build_pp_summary(daily_df: pd.DataFrame) -> pd.DataFrame:
    hours = (
        daily_df
        .groupby(["Name", "Crew", "Pay Period"], as_index=False)["Paid Hours"]
        .sum()
        .rename(columns={"Paid Hours": "Total Hours"})
    )
    day_counts = (
        daily_df
        .groupby(["Name", "Crew", "Pay Period", "Status"], as_index=False)
        .size()
        .pivot_table(index=["Name", "Crew", "Pay Period"],
                     columns="Status", values="size", fill_value=0)
        .reset_index()
    )
    merged = hours.merge(day_counts, on=["Name", "Crew", "Pay Period"])
    for col in ("Watch", "Admin", "Off"):
        if col not in merged.columns:
            merged[col] = 0
    return merged[["Name", "Crew", "Pay Period", "Total Hours",
                    "Watch", "Admin", "Off"]].rename(columns={
        "Watch": "Watch Days", "Admin": "Admin Days", "Off": "Off Days",
    })


def build_coverage_check(daily_df: pd.DataFrame) -> pd.DataFrame:
    """PAX per shift per day, plus crews-on-duty count and coverage flag."""
    watch = daily_df[daily_df["Status"] == "Watch"]

    pax = (
        watch
        .groupby(["Date", "Pay Period", "Shift"], as_index=False)
        .size()
        .pivot_table(index=["Date", "Pay Period"],
                     columns="Shift", values="size", fill_value=0)
        .reset_index()
    )

    crews_on = (
        watch
        .groupby(["Date", "Pay Period"])["Crew"]
        .nunique()
        .reset_index()
        .rename(columns={"Crew": "Crews On Duty"})
    )

    merged = pax.merge(crews_on, on=["Date", "Pay Period"])

    if STAFFING_MODE == "3x8":
        shift_cols = list(SHIFTS_3x8.keys())
    else:
        shift_cols = list(SHIFTS_2x12.keys())

    for col in shift_cols:
        if col not in merged.columns:
            merged[col] = 0

    # Rename shift columns to include "PAX"
    rename_map = {s: f"{s} PAX" for s in shift_cols}
    merged = merged.rename(columns=rename_map)
    pax_cols = [f"{s} PAX" for s in shift_cols]

    merged["Coverage OK"] = merged[pax_cols].min(axis=1) >= PAX_PER_SHIFT

    return merged[["Date", "Pay Period"] + pax_cols +
                  ["Crews On Duty", "Coverage OK"]]


def export_excel(daily_df: pd.DataFrame, weekly_df: pd.DataFrame,
                 pp_df: pd.DataFrame, coverage_df: pd.DataFrame) -> None:
    with pd.ExcelWriter(OUTPUT_FILE, engine="xlsxwriter") as writer:
        daily_df.to_excel(writer, sheet_name="Daily_Assignments", index=False)
        weekly_df.to_excel(writer, sheet_name="Weekly_Summary", index=False)
        pp_df.to_excel(writer, sheet_name="PayPeriod_Summary", index=False)
        coverage_df.to_excel(writer, sheet_name="Coverage_Check", index=False)
        daily_df.to_excel(writer, sheet_name="Pivot_Source", index=False)


def validate(daily_df: pd.DataFrame, pp_df: pd.DataFrame,
             coverage_df: pd.DataFrame) -> bool:
    ok = True

    # 80h cap
    over_80 = pp_df[pp_df["Total Hours"] > MAX_HOURS_PER_PAY_PERIOD]
    if len(over_80) > 0:
        print(f"FAIL: {len(over_80)} person-periods exceed {MAX_HOURS_PER_PAY_PERIOD}h")
        print(over_80.to_string(index=False))
        ok = False
    else:
        print(f"PASS: No one exceeds {MAX_HOURS_PER_PAY_PERIOD}h per pay period")

    # Coverage
    bad_cov = coverage_df[~coverage_df["Coverage OK"]]
    if len(bad_cov) > 0:
        print(f"FAIL: {len(bad_cov)} days with insufficient shift coverage (<{PAX_PER_SHIFT} PAX)")
        print(bad_cov.to_string(index=False))
        ok = False
    else:
        print(f"PASS: Every shift has ≥{PAX_PER_SHIFT} PAX every day")

    # Stats
    hours_stats = pp_df["Total Hours"]
    print(f"Hours per person per PP: min={hours_stats.min()}, "
          f"max={hours_stats.max()}, avg={hours_stats.mean():.1f}")

    total_admin = pp_df["Admin Days"].sum()
    print(f"Total admin days: {total_admin}")

    return ok


def main() -> None:
    print(f"Generating COA2 schedule ({STAFFING_MODE} mode)")
    print(f"  Date range: {START_DATE} to {END_DATE}")
    print(f"  Crews: {len(CREWS)} ({sum(len(m) for m in CREWS.values())} personnel)")
    print()

    daily_df = build_daily_schedule()
    weekly_df = build_weekly_summary(daily_df)
    pp_df = build_pp_summary(daily_df)
    coverage_df = build_coverage_check(daily_df)

    validate(daily_df, pp_df, coverage_df)
    print()

    export_excel(daily_df, weekly_df, pp_df, coverage_df)
    print(f"Excel schedule generated: {OUTPUT_FILE}")


if __name__ == "__main__":
    main()
