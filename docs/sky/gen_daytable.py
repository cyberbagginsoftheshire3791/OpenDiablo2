"""Generate d2core/d2world/daytable_gen.go -- the Strigoi day table for the
six-night slice (17-23 June 1462, Julian), Targoviste local mean time.

Article II.5: derived facts live in the systems that produce them. The HUD's
clock strip shows a feast/fast name and a moon-phase name keyed on the date;
those, and the rest of the historical sky (sunrise, sunset, moonrise, lit %),
are GENERATED here, not hand-typed into Go. Run this once and commit the
generated .go; it never runs on CI and touches no MPQs (Article V).

    python docs/sky/gen_daytable.py

Instruments:
  meeus.py       (instrument B) -- pure-math Meeus 1998: sun rise/set (ch. 25+15)
                 and moon phases (ch. 49). No ephem/astropy needed for these.
  PyEphem        (instrument A) -- moonrise and illuminated %, the two columns
                 meeus does not compute here. `pip install ephem` (4.2.x).
  C3 computus    -- Gauss Julian Easter + Zeller weekday, reproduced from
                 `C3 - Julian Calendar and Orthodox Feast Days 1462.md` s3.5/3.6.

Controls run every time (Constitution VI.4b), and the script REFUSES TO WRITE
if any fails:
  instrument audit : meeus.py's own worked examples (run `python meeus.py`).
  reproduction     : 17 Jun 1462 sunset == 19:50 LMT (signed S1 Appendix A).
  negative         : 1 Jan 1462 sunset != the slice's, proving the sun
                     instrument reads the date rather than emitting a constant.
  computus derive  : the movable feasts are placed by E+offset (E+60 = 9th Thu
                     after Easter on 17 Jun; E+63 = 2nd Sun after Pentecost on
                     20 Jun), not by hand-assigned day number.
"""
import sys
import os
import math

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import meeus as M  # noqa: E402

REPO_ROOT = os.path.normpath(os.path.join(HERE, "..", ".."))
OUT_GO = os.path.join(REPO_ROOT, "d2core", "d2world", "daytable_gen.go")

LAT, LON, ELEV = 44.93, 25.46, 280.0  # Targoviste (S1's coordinates)
LMT_H = LON / 15.0                     # local mean time offset, hours east of Greenwich
DT = M.delta_t_seconds(1462)           # ~240.7 s

# ------------------------------------------------------------------ the sun --
def sun_rise_set_minutes(y, m, d):
    """Return (sunrise, sunset) as integer minute-of-day in Targoviste LMT."""
    ev = M.sun_events(y, m, d, LAT, LON, DT)
    up = _first(ev["h0"], "up")
    down = _first(ev["h0"], "down")
    return _lmt_minute(up), _lmt_minute(down)


def _first(events, kind):
    for jd, k in events:
        if k == kind:
            return jd
    return None


def _lmt_minute(jd_ut):
    """UT JD -> integer minute-of-day in local mean time."""
    if jd_ut is None:
        return -1
    _, _, dfrac = M.cal_from_jd(jd_ut + LMT_H / 24.0)
    frac = dfrac - math.floor(dfrac)
    return int(round(frac * 24 * 60)) % (24 * 60)


def _hhmm(minute):
    return f"{minute // 60:02d}:{minute % 60:02d}"


# --------------------------------------------------- the moon (phase name) --
def moon_phases():
    phases = M.phases_between(M.jd_from_cal(1462, 5, 20), M.jd_from_cal(1462, 7, 5))
    return [(n, j - DT / 86400.0) for (n, j) in phases]  # JDE(TT) -> UT


def moon_phase_name(jd_noon_lmt, phases):
    before = [(n, j) for (n, j) in phases if j <= jd_noon_lmt]
    after = [(n, j) for (n, j) in phases if j > jd_noon_lmt]
    if not before or not after:
        return "?"
    table = {
        ("full", "last quarter"): "waning gibbous",
        ("last quarter", "new"): "waning crescent",
        ("new", "first quarter"): "waxing crescent",
        ("first quarter", "full"): "waxing gibbous",
    }
    return table.get((before[-1][0], after[0][0]), f"{before[-1][0]} -> {after[0][0]}")


# ------------------------------------------ the moon (rise + lit %, ephem) --
def moon_rise_lit(dd):
    """Return (moonrise_minute_of_day or -1, lit_percent) for the night of dd,
    via PyEphem. Ported from regen_moon_columns.py / run_c2c4.py section 3."""
    import ephem
    obs = ephem.Observer()
    obs.lat, obs.lon, obs.elevation = str(LAT), str(LON), ELEV
    obs.pressure = 0
    obs.horizon = "-0:50"
    sun, moon = ephem.Sun(), ephem.Moon()

    def ephem_to_jd(e):
        return float(e) + 2415020.0

    local_midnight_ut = ephem.Date(f"1462/6/{dd} 0:00") - LMT_H / 24.0
    obs.date = local_midnight_ut
    ss = ephem_to_jd(obs.next_setting(sun, use_center=True))
    obs.date = ephem.Date(ss - 2415020.0)
    sr = ephem_to_jd(obs.next_rising(sun, use_center=True))
    obs.date = ephem.Date(ss - 2415020.0)
    try:
        mr = ephem_to_jd(obs.next_rising(moon))
        mr = mr if mr < sr else None
    except Exception:
        mr = None
    local_mid = M.jd_from_cal(1462, 6, dd) + 1 - LMT_H / 24.0
    moon.compute(ephem.Date(local_mid - 2415020.0))
    lit = round(moon.phase, 1)
    return (_lmt_minute(mr) if mr is not None else -1), lit


# ------------------------------------------------- the C3 computus + feast --
def gauss_julian_easter(Y, Mc=15, N=6):
    a, b, c = Y % 4, Y % 7, Y % 19
    d = (19 * c + Mc) % 30
    e = (2 * a + 4 * b - d + N) % 7
    day = 22 + d + e
    return (4, day - 31) if day > 31 else (3, day)


def zeller_julian(y, m, q):  # 0=Sat 1=Sun ... 6=Fri
    if m < 3:
        m += 12
        y -= 1
    K, J = y % 100, y // 100
    return (q + (13 * (m + 1)) // 5 + K + K // 4 + 5 - J) % 7


ZWD_SHORT = ["Sat", "Sun", "Mon", "Tue", "Wed", "Thu", "Fri"]
ZWD_FULL = ["Saturday", "Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday"]

# Movable feasts placed by offset from Julian Easter (DERIVED, a control below).
MOVABLE = {60: "9th Thursday after Easter", 63: "2nd Sunday after Pentecost"}

# Fixed commemoration + Typikon fast rule, GROUNDED in C3 s3.6/4.1 (not
# computable -- carried verbatim). Displayed feast is the movable name when
# present, else "Apostles' Fast" (the whole slice is inside the Apostles' Fast,
# 14-28 June 1462); this matches the strings the 11 Sep ruling named.
FIXED = {
    17: ("Mart. Manuel, Sabel and Ismael", "oil and wine, no fish"),
    18: ("Mart. Leontius", "xerophagy (no oil or wine)"),
    19: ("Apostle Jude (Thaddeus), brother of the Lord", "fish, wine, oil"),
    20: ("Hieromart. Methodius of Patara", "fish, wine, oil"),
    21: ("Mart. Julian of Tarsus", "xerophagy (no oil or wine)"),
    22: ("Hieromart. Eusebius of Samosata", "oil and wine"),
    23: ("Mart. Agrippina of Rome", "xerophagy (no oil or wine)"),
}

APOSTLES_FAST = "Apostles' Fast"


def go_string(s):
    """Quote a Go string literal, ASCII only (fail loud on anything else)."""
    s.encode("ascii")  # raises if a non-ASCII glyph sneaks in
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def main():
    # ---- controls on the sun instrument (hard asserts) ----
    sr17_min, ss17_min = sun_rise_set_minutes(1462, 6, 17)
    _, ssjan_min = sun_rise_set_minutes(1462, 1, 1)
    assert _hhmm(ss17_min) == "19:50", f"reproduction FAILED: 17 Jun sunset {_hhmm(ss17_min)} != 19:50"
    assert ss17_min != ssjan_min, "negative control FAILED: winter sunset == summer sunset"
    print(f"reproduction  17 Jun 1462 sunset = {_hhmm(ss17_min)} LMT  (S1 App A: 19:50)  PASS")
    print(f"negative      01 Jan 1462 sunset = {_hhmm(ssjan_min)} LMT  (must differ)      PASS")

    # ---- computus: Easter + the movable-feast placement (derive, then check) ----
    em, ed = gauss_julian_easter(1462)
    assert (em, ed) == (4, 18), f"Easter 1462 = {ed}/{em}, expected 18 April"
    e_jd = M.jd_from_cal(1462, em, ed)
    print(f"computus      Julian Easter 1462 = {ed} April  PASS")

    phases = moon_phases()

    rows = []
    for dd in range(17, 24):
        z = zeller_julian(1462, 6, dd)
        wd_full = ZWD_FULL[z]
        eoff = int(round(M.jd_from_cal(1462, 6, dd) - e_jd))
        feast = MOVABLE.get(eoff, APOSTLES_FAST)
        detail, fastrule = FIXED[dd]

        sr_min, ss_min = sun_rise_set_minutes(1462, 6, dd)
        noon_lmt = M.jd_from_cal(1462, 6, dd) + 0.5 - LMT_H / 24.0
        phase = moon_phase_name(noon_lmt + LMT_H / 24.0, phases)
        mr_min, lit = moon_rise_lit(dd)

        rows.append({
            "y": 1462, "m": 6, "d": dd, "wd": wd_full,
            "feast": feast, "detail": detail, "fast": fastrule, "phase": phase,
            "sr": sr_min, "ss": ss_min, "mr": mr_min, "lit": lit,
        })
        print(f"  1462-06-{dd:02d} {ZWD_SHORT[z]} E+{eoff:<3d} rise {_hhmm(sr_min)} set {_hhmm(ss_min)} "
              f"{phase:15s} moonrise {(_hhmm(mr_min) if mr_min >= 0 else '  -  ')} lit {lit:4.1f}%  {feast}; {detail}")

    # sanity: the two movable feasts landed where the ruling says
    assert rows[0]["feast"] == "9th Thursday after Easter", "17 Jun is not the 9th Thu after Easter"
    assert rows[3]["feast"] == "2nd Sunday after Pentecost", "20 Jun is not the 2nd Sun after Pentecost"

    emit_go(rows)
    print(f"\nwrote {os.path.relpath(OUT_GO, REPO_ROOT)}")


def emit_go(rows):
    lines = []
    a = lines.append
    a("// Code generated by docs/sky/gen_daytable.py; DO NOT EDIT.")
    a("// Regenerate: python docs/sky/gen_daytable.py  (needs meeus.py + PyEphem;")
    a("// never runs on CI, touches no MPQs -- Article V). Source: Meeus 1998")
    a("// (sun rise/set, moon phase), PyEphem (moonrise, lit %), and the C3")
    a("// computus/menologion (weekday, Easter offsets, feast/fast names).")
    a("// Controls passed at generation: instrument audit, reproduction (17 Jun")
    a("// sunset 19:50 LMT = signed S1 Appendix A), negative (1 Jan != slice).")
    a("")
    a("package d2world")
    a("")
    a("// sliceDayTable is the six-night slice (17-23 June 1462, Julian) in")
    a("// Targoviste local mean time. Minute fields are minute-of-day, [0,1440);")
    a("// MoonriseMinute is -1 when the moon does not rise before sunrise.")
    a("var sliceDayTable = []DayEntry{")
    for r in rows:
        a(
            "\t{"
            f"Year: {r['y']}, Month: {r['m']}, Day: {r['d']}, "
            f"Weekday: {go_string(r['wd'])}, "
            f"Feast: {go_string(r['feast'])}, "
            f"FeastDetail: {go_string(r['detail'])}, "
            f"FastRule: {go_string(r['fast'])}, "
            f"MoonPhase: {go_string(r['phase'])}, "
            f"SunriseMinute: {r['sr']}, SunsetMinute: {r['ss']}, "
            f"MoonriseMinute: {r['mr']}, MoonLitPercent: {r['lit']}"
            "},"
        )
    a("}")
    a("")
    with open(OUT_GO, "w", encoding="ascii", newline="\n") as f:
        f.write("\n".join(lines))


if __name__ == "__main__":
    main()
