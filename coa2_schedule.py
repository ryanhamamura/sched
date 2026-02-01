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

# 3-2-2-3-2-2: 7 on / 7 off across a 14-day pay period
PATTERN_A = [True]*3 + [False]*2 + [True]*2 + [False]*3 + [True]*2 + [False]*2
PATTERN_B = [not x for x in PATTERN_A]  # complement — together they cover every day

# 2 people per shift (A/B pair), N shifts per mode
PEOPLE_PER_PP = {"3x8": 6, "2x12": 4}

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
    ppp = PEOPLE_PER_PP[STAFFING_MODE]
    rows = []

    cursor = 0
    for _pp_idx, pp_label, pp_start, pp_end in pay_periods:
        # Pick next batch of people from the pool
        assigned = [POOL[(cursor + i) % len(POOL)] for i in range(ppp)]
        cursor += ppp

        # Pair them up per shift: (pattern-A person, pattern-B person)
        shift_pairs = []
        for s_idx, shift in enumerate(shift_names):
            person_a = assigned[s_idx * 2]
            person_b = assigned[s_idx * 2 + 1]
            shift_pairs.append((shift, person_a, person_b))

        day = pp_start
        day_idx = 0
        while day <= pp_end:
            week_num = day.isocalendar()[1]
            for shift, person_a, person_b in shift_pairs:
                if day_idx < len(PATTERN_A) and PATTERN_A[day_idx]:
                    rows.append([day, pp_label, week_num, person_a, shift, h])
                else:
                    rows.append([day, pp_label, week_num, person_a, "Normal Duty", h])
                if day_idx < len(PATTERN_B) and PATTERN_B[day_idx]:
                    rows.append([day, pp_label, week_num, person_b, shift, h])
                else:
                    rows.append([day, pp_label, week_num, person_b, "Normal Duty", h])
            day += timedelta(days=1)
            day_idx += 1

    return pd.DataFrame(rows, columns=[
        "Date", "Pay Period", "Week", "Name", "Shift", "Hours",
    ])


def _duty_only(df: pd.DataFrame) -> pd.DataFrame:
    """Filter out Normal Duty rows — only real shift assignments."""
    return df[df["Shift"] != "Normal Duty"]


def build_weekly_summary(daily_df: pd.DataFrame) -> pd.DataFrame:
    return (
        _duty_only(daily_df)
        .groupby(["Name", "Week"], as_index=False)["Hours"]
        .sum()
        .rename(columns={"Hours": "Weekly Hours"})
    )


def build_pp_summary(daily_df: pd.DataFrame) -> pd.DataFrame:
    duty_df = _duty_only(daily_df)
    hours = (
        duty_df
        .groupby(["Name", "Pay Period"], as_index=False)["Hours"]
        .sum()
        .rename(columns={"Hours": "Total Hours"})
    )
    shifts = (
        duty_df
        .groupby(["Name", "Pay Period"], as_index=False)
        .size()
        .rename(columns={"size": "Shifts"})
    )
    result = hours.merge(shifts, on=["Name", "Pay Period"])
    result["Normal Duty Hours"] = MAX_HOURS_PER_PAY_PERIOD - result["Total Hours"]
    return result


def build_fairness_summary(daily_df: pd.DataFrame) -> pd.DataFrame:
    duty_df = _duty_only(daily_df)
    total_shifts = (
        duty_df
        .groupby("Name", as_index=False)
        .size()
        .rename(columns={"size": "Total Shifts"})
    )
    total_hours = (
        duty_df
        .groupby("Name", as_index=False)["Hours"]
        .sum()
        .rename(columns={"Hours": "Total Hours"})
    )
    pp_count = (
        duty_df
        .groupby("Name", as_index=False)["Pay Period"]
        .nunique()
        .rename(columns={"Pay Period": "PPs Worked"})
    )
    return total_shifts.merge(total_hours, on="Name").merge(pp_count, on="Name")


def build_coverage_check(daily_df: pd.DataFrame) -> pd.DataFrame:
    pax = (
        _duty_only(daily_df)
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

    # Fairness: PP count spread should be ≤ 1
    pp_spread = fairness_df["PPs Worked"].max() - fairness_df["PPs Worked"].min()
    if pp_spread > 1:
        print(f"FAIL: PP assignment spread is {pp_spread} (max allowed: 1)")
        ok = False
    else:
        print(f"PASS: PP assignment spread is {pp_spread} (≤ 1)")

    # Stats
    hours_stats = pp_df["Total Hours"]
    print(f"\nHours per person per PP: min={hours_stats.min()}, "
          f"max={hours_stats.max()}, avg={hours_stats.mean():.1f}")
    print(f"PPs worked per person: min={fairness_df['PPs Worked'].min()}, "
          f"max={fairness_df['PPs Worked'].max()}")
    print(f"Total people used: {len(fairness_df)} / {len(POOL)}")

    return ok


def main() -> None:
    ppp = PEOPLE_PER_PP[STAFFING_MODE]
    pay_periods = make_pay_periods(START_DATE, END_DATE)
    total_slots = ppp * len(pay_periods)

    print(f"Generating COA2 schedule ({STAFFING_MODE} mode)")
    print(f"  Date range: {START_DATE} to {END_DATE}")
    print(f"  Pool: {len(POOL)} personnel, {ppp} per PP ({total_slots} total slots)")
    print(f"  On/off pattern: 3-2-2-3-2-2 (7 on / 7 off per PP)")
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
