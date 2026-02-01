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

POOL = [f"P-{i:02d}" for i in range(1, 36)]  # 35 people
PAX_PER_SHIFT = 1

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


def hours_per_shift() -> int:
    return 8 if STAFFING_MODE == "3x8" else 12


def build_daily_schedule() -> pd.DataFrame:
    pay_periods = make_pay_periods(START_DATE, END_DATE)
    h = hours_per_shift()
    shift_names = list(SHIFTS_3x8.keys()) if STAFFING_MODE == "3x8" else list(SHIFTS_2x12.keys())
    rows = []

    # Track hours per person per pay period for 80h cap
    cursor = 0

    for pp_idx, pp_label, pp_start, pp_end in pay_periods:
        person_hours: dict[str, int] = {}

        day = pp_start
        while day <= pp_end:
            week_num = day.isocalendar()[1]

            for shift in shift_names:
                # Find next person who won't exceed 80h cap
                attempts = 0
                while attempts < len(POOL):
                    person = POOL[cursor % len(POOL)]
                    accumulated = person_hours.get(person, 0)
                    if accumulated + h <= MAX_HOURS_PER_PAY_PERIOD:
                        person_hours[person] = accumulated + h
                        cursor += 1
                        break
                    cursor += 1
                    attempts += 1

                rows.append([day, pp_label, week_num, person, shift, h])

            day += timedelta(days=1)

    return pd.DataFrame(rows, columns=[
        "Date", "Pay Period", "Week", "Name", "Shift", "Hours",
    ])


def build_weekly_summary(daily_df: pd.DataFrame) -> pd.DataFrame:
    return (
        daily_df
        .groupby(["Name", "Week"], as_index=False)["Hours"]
        .sum()
        .rename(columns={"Hours": "Weekly Hours"})
    )


def build_pp_summary(daily_df: pd.DataFrame) -> pd.DataFrame:
    hours = (
        daily_df
        .groupby(["Name", "Pay Period"], as_index=False)["Hours"]
        .sum()
        .rename(columns={"Hours": "Total Hours"})
    )
    shifts = (
        daily_df
        .groupby(["Name", "Pay Period"], as_index=False)
        .size()
        .rename(columns={"size": "Shifts"})
    )
    return hours.merge(shifts, on=["Name", "Pay Period"])


def build_fairness_summary(daily_df: pd.DataFrame) -> pd.DataFrame:
    total_shifts = (
        daily_df
        .groupby("Name", as_index=False)
        .size()
        .rename(columns={"size": "Total Shifts"})
    )
    total_hours = (
        daily_df
        .groupby("Name", as_index=False)["Hours"]
        .sum()
        .rename(columns={"Hours": "Total Hours"})
    )
    return total_shifts.merge(total_hours, on="Name")


def build_coverage_check(daily_df: pd.DataFrame) -> pd.DataFrame:
    pax = (
        daily_df
        .groupby(["Date", "Pay Period", "Shift"], as_index=False)
        .size()
        .pivot_table(index=["Date", "Pay Period"],
                     columns="Shift", values="size", fill_value=0)
        .reset_index()
    )

    shift_cols = list(SHIFTS_3x8.keys()) if STAFFING_MODE == "3x8" else list(SHIFTS_2x12.keys())
    for col in shift_cols:
        if col not in pax.columns:
            pax[col] = 0

    rename_map = {s: f"{s} PAX" for s in shift_cols}
    pax = pax.rename(columns=rename_map)
    pax_cols = [f"{s} PAX" for s in shift_cols]

    pax["Coverage OK"] = pax[pax_cols].min(axis=1) >= PAX_PER_SHIFT

    return pax[["Date", "Pay Period"] + pax_cols + ["Coverage OK"]]


def export_excel(daily_df: pd.DataFrame, weekly_df: pd.DataFrame,
                 pp_df: pd.DataFrame, fairness_df: pd.DataFrame,
                 coverage_df: pd.DataFrame) -> None:
    with pd.ExcelWriter(OUTPUT_FILE, engine="xlsxwriter") as writer:
        daily_df.to_excel(writer, sheet_name="Daily_Assignments", index=False)
        weekly_df.to_excel(writer, sheet_name="Weekly_Summary", index=False)
        pp_df.to_excel(writer, sheet_name="PayPeriod_Summary", index=False)
        fairness_df.to_excel(writer, sheet_name="Fairness_Summary", index=False)
        coverage_df.to_excel(writer, sheet_name="Coverage_Check", index=False)


def validate(daily_df: pd.DataFrame, pp_df: pd.DataFrame,
             fairness_df: pd.DataFrame, coverage_df: pd.DataFrame) -> bool:
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

    # Fairness: max-min shift spread should be ≤ 1
    spread = fairness_df["Total Shifts"].max() - fairness_df["Total Shifts"].min()
    if spread > 1:
        print(f"FAIL: Shift distribution spread is {spread} (max allowed: 1)")
        ok = False
    else:
        print(f"PASS: Shift distribution spread is {spread} (≤ 1)")

    # Stats
    hours_stats = pp_df["Total Hours"]
    print(f"\nHours per person per PP: min={hours_stats.min()}, "
          f"max={hours_stats.max()}, avg={hours_stats.mean():.1f}")
    print(f"Shifts per person overall: min={fairness_df['Total Shifts'].min()}, "
          f"max={fairness_df['Total Shifts'].max()}, "
          f"avg={fairness_df['Total Shifts'].mean():.1f}")

    return ok


def main() -> None:
    print(f"Generating COA2 schedule ({STAFFING_MODE} mode)")
    print(f"  Date range: {START_DATE} to {END_DATE}")
    print(f"  Pool: {len(POOL)} personnel, {PAX_PER_SHIFT} per shift")
    print()

    daily_df = build_daily_schedule()
    weekly_df = build_weekly_summary(daily_df)
    pp_df = build_pp_summary(daily_df)
    fairness_df = build_fairness_summary(daily_df)
    coverage_df = build_coverage_check(daily_df)

    validate(daily_df, pp_df, fairness_df, coverage_df)
    print()

    export_excel(daily_df, weekly_df, pp_df, fairness_df, coverage_df)
    print(f"Excel schedule generated: {OUTPUT_FILE}")


if __name__ == "__main__":
    main()
