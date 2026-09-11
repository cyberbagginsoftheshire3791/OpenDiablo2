"""Independent instrument B for the C2/C4 precision pass: hand-coded Meeus,
*Astronomical Algorithms* (2nd ed., 1998) — ch. 7 (Julian Day), ch. 12 (sidereal time),
ch. 22 (obliquity), ch. 25 (solar coordinates, low accuracy ~0.01 deg), ch. 49 (phases of the Moon).
Written 30 Aug 2026 for Project Strigoi, deliberately WITHOUT importing ephem/astropy, so that
its numbers are independent of instrument A (PyEphem) and instrument C (astropy/ERFA).
Every formula is cited to its Meeus chapter so a reader can check it against the book.
"""
import math

D2R = math.pi / 180.0
R2D = 180.0 / math.pi


# ---------- ch. 7: Julian Day <-> calendar (Julian calendar before 1582-10-15) ----------
def jd_from_cal(y, m, d, julian=None):
    """Meeus 7.1. `d` may be fractional (UT). julian=None -> auto (Julian calendar before 1582-10-15)."""
    if julian is None:
        julian = (y, m, d) < (1582, 10, 15)
    if m <= 2:
        y -= 1
        m += 12
    if julian:
        B = 0
    else:
        A = y // 100
        B = 2 - A + A // 4
    return math.floor(365.25 * (y + 4716)) + math.floor(30.6001 * (m + 1)) + d + B - 1524.5


def cal_from_jd(jd):
    """Meeus 7 inverse. Returns (y, m, d_fractional); Julian calendar for JD < 2299161 (1582-10-15)."""
    jd += 0.5
    Z = math.floor(jd)
    F = jd - Z
    if Z < 2299161:
        A = Z
    else:
        alpha = math.floor((Z - 1867216.25) / 36524.25)
        A = Z + 1 + alpha - alpha // 4
    B = A + 1524
    C = math.floor((B - 122.1) / 365.25)
    D = math.floor(365.25 * C)
    E = math.floor((B - D) / 30.6001)
    day = B - D - math.floor(30.6001 * E) + F
    month = E - 1 if E < 14 else E - 13
    year = C - 4716 if month > 2 else C - 4715
    return year, month, day


WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]


def weekday_from_jd(jd):
    """Meeus 7: (JD + 1.5) mod 7 -> 0 = Sunday."""
    return WEEKDAYS[int(math.floor(jd + 1.5)) % 7]


# ---------- Delta T (TT - UT), NASA/Espenak-Meeus polynomial for 500 <= y < 1600 ----------
def delta_t_seconds(year):
    """Espenak & Meeus polynomial (eclipse.gsfc.nasa.gov/SEcat5/deltatpoly.html), 500-1600 branch.
    VERIFY: the branch was re-fetched from the NASA page on 30 Aug 2026 (see run log)."""
    u = (year - 1000) / 100.0
    return (1574.2 - 556.01 * u + 71.23472 * u ** 2 + 0.319781 * u ** 3
            - 0.8503463 * u ** 4 - 0.005050998 * u ** 5 + 0.0083572073 * u ** 6)


# ---------- ch. 12: mean sidereal time at Greenwich ----------
def gmst_deg(jd_ut):
    """Meeus 12.4 (mean sidereal time at Greenwich, degrees)."""
    T = (jd_ut - 2451545.0) / 36525.0
    th = (280.46061837 + 360.98564736629 * (jd_ut - 2451545.0)
          + 0.000387933 * T * T - T ** 3 / 38710000.0)
    return th % 360.0


# ---------- ch. 22 (low precision) + ch. 25: apparent solar coordinates ----------
def sun_apparent(jde):
    """Meeus ch. 25 'low accuracy' (0.01 deg). Returns (ra_deg, dec_deg, R_au, apparent_lon_deg)."""
    T = (jde - 2451545.0) / 36525.0
    L0 = (280.46646 + 36000.76983 * T + 0.0003032 * T * T) % 360.0
    M = (357.52911 + 35999.05029 * T - 0.0001537 * T * T) % 360.0
    Mr = M * D2R
    C = ((1.914602 - 0.004817 * T - 0.000014 * T * T) * math.sin(Mr)
         + (0.019993 - 0.000101 * T) * math.sin(2 * Mr)
         + 0.000289 * math.sin(3 * Mr))
    true_lon = L0 + C
    nu = M + C
    e = 0.016708634 - 0.000042037 * T - 0.0000001267 * T * T
    R = 1.000001018 * (1 - e * e) / (1 + e * math.cos(nu * D2R))
    Omega = 125.04 - 1934.136 * T
    lam = true_lon - 0.00569 - 0.00478 * math.sin(Omega * D2R)
    eps0 = (23 + 26 / 60.0 + 21.448 / 3600.0) - (46.8150 * T + 0.00059 * T * T - 0.001813 * T ** 3) / 3600.0
    eps = eps0 + 0.00256 * math.cos(Omega * D2R)
    lr, er = lam * D2R, eps * D2R
    ra = math.atan2(math.cos(er) * math.sin(lr), math.cos(lr)) * R2D % 360.0
    dec = math.asin(math.sin(er) * math.sin(lr)) * R2D
    return ra, dec, R, lam % 360.0


def sun_altitude(jd_ut, lat_deg, lon_deg_east, delta_t_s):
    """Geometric altitude of the Sun's centre (no refraction), degrees, at UT instant jd_ut."""
    jde = jd_ut + delta_t_s / 86400.0
    ra, dec, _, _ = sun_apparent(jde)
    lst = (gmst_deg(jd_ut) + lon_deg_east) % 360.0
    H = (lst - ra) * D2R
    phi, d = lat_deg * D2R, dec * D2R
    return math.asin(math.sin(phi) * math.sin(d) + math.cos(phi) * math.cos(d) * math.cos(H)) * R2D


def find_crossing(alt_fn, jd_lo, jd_hi, target, rising):
    """Bisection for alt_fn(jd) == target between jd_lo and jd_hi; rising=True means alt increasing."""
    f_lo = alt_fn(jd_lo) - target
    f_hi = alt_fn(jd_hi) - target
    if (f_lo < 0) == (f_hi < 0):
        return None
    for _ in range(60):
        mid = 0.5 * (jd_lo + jd_hi)
        f_mid = alt_fn(mid) - target
        if (f_mid < 0) == (f_lo < 0):
            jd_lo, f_lo = mid, f_mid
        else:
            jd_hi, f_hi = mid, f_mid
    return 0.5 * (jd_lo + jd_hi)


def sun_events(y, m, d, lat, lon, delta_t_s, step_min=10):
    """For the local (Julian-calendar) date, scan the sun's altitude from local midnight to local
    midnight (in local mean time = UT + lon/15 h) and return crossing times (UT JD) for
    -0.8333 (rise/set, Meeus 15: h0 = -0 deg 50'), -6, -12, -18 (twilights)."""
    lmt_offset = lon / 15.0 / 24.0  # days
    jd0 = jd_from_cal(y, m, d) - lmt_offset  # local mean midnight, expressed in UT
    alt = lambda jd: sun_altitude(jd, lat, lon, delta_t_s)
    n = int(24 * 60 / step_min)
    grid = [jd0 + i * step_min / 1440.0 for i in range(n + 1)]
    alts = [alt(j) for j in grid]
    out = {}
    for target, name in ((-0.8333, "h0"), (-6.0, "civil"), (-12.0, "nautical"), (-18.0, "astro")):
        crossings = []
        for i in range(n):
            a0, a1 = alts[i] - target, alts[i + 1] - target
            if (a0 < 0) != (a1 < 0):
                jd = find_crossing(alt, grid[i], grid[i + 1], target, a1 > a0)
                crossings.append((jd, "up" if a1 > a0 else "down"))
        out[name] = crossings
    return out


# ---------- ch. 49: phases of the Moon ----------
def moon_phase_jde(k):
    """Meeus ch. 49. k integer = new moon; +0.25 first quarter; +0.5 full; +0.75 last quarter.
    Returns JDE (Terrestrial/Dynamical Time)."""
    frac = round((k % 1) * 4) / 4.0
    T = k / 1236.85
    JDE = (2451550.09766 + 29.530588861 * k + 0.00015437 * T ** 2
           - 0.000000150 * T ** 3 + 0.00000000073 * T ** 4)
    E = 1 - 0.002516 * T - 0.0000074 * T ** 2
    M = (2.5534 + 29.10535670 * k - 0.0000014 * T ** 2 - 0.00000011 * T ** 3) * D2R
    Mp = (201.5643 + 385.81693528 * k + 0.0107582 * T ** 2 + 0.00001238 * T ** 3 - 0.000000058 * T ** 4) * D2R
    F = (160.7108 + 390.67050284 * k - 0.0016118 * T ** 2 - 0.00000227 * T ** 3 + 0.000000011 * T ** 4) * D2R
    Om = (124.7746 - 1.56375588 * k + 0.0020672 * T ** 2 + 0.00000215 * T ** 3) * D2R
    A = [
        299.77 + 0.107408 * k - 0.009173 * T ** 2,
        251.88 + 0.016321 * k,
        251.83 + 26.651886 * k,
        349.42 + 36.412478 * k,
        84.66 + 18.206239 * k,
        141.74 + 53.303771 * k,
        207.14 + 2.453732 * k,
        154.84 + 7.306860 * k,
        34.52 + 27.261239 * k,
        207.19 + 0.121824 * k,
        291.34 + 1.844379 * k,
        161.72 + 24.198154 * k,
        239.56 + 25.513099 * k,
        331.55 + 3.592518 * k,
    ]
    s = math.sin
    if frac in (0.0, 0.5):
        if frac == 0.0:
            c = (-0.40720 * s(Mp) + 0.17241 * E * s(M) + 0.01608 * s(2 * Mp) + 0.01039 * s(2 * F)
                 + 0.00739 * E * s(Mp - M) - 0.00514 * E * s(Mp + M) + 0.00208 * E * E * s(2 * M)
                 - 0.00111 * s(Mp - 2 * F) - 0.00057 * s(Mp + 2 * F) + 0.00056 * E * s(2 * Mp + M)
                 - 0.00042 * s(3 * Mp) + 0.00042 * E * s(M + 2 * F) + 0.00038 * E * s(M - 2 * F)
                 - 0.00024 * E * s(2 * Mp - M) - 0.00017 * s(Om) - 0.00007 * s(Mp + 2 * M)
                 + 0.00004 * s(2 * Mp - 2 * F) + 0.00004 * s(3 * M) + 0.00003 * s(Mp + M - 2 * F)
                 + 0.00003 * s(2 * Mp + 2 * F) - 0.00003 * s(Mp + M + 2 * F) + 0.00003 * s(Mp - M + 2 * F)
                 - 0.00002 * s(Mp - M - 2 * F) - 0.00002 * s(3 * Mp + M) + 0.00002 * s(4 * Mp))
        else:
            c = (-0.40614 * s(Mp) + 0.17302 * E * s(M) + 0.01614 * s(2 * Mp) + 0.01043 * s(2 * F)
                 + 0.00734 * E * s(Mp - M) - 0.00515 * E * s(Mp + M) + 0.00209 * E * E * s(2 * M)
                 - 0.00111 * s(Mp - 2 * F) - 0.00057 * s(Mp + 2 * F) + 0.00056 * E * s(2 * Mp + M)
                 - 0.00042 * s(3 * Mp) + 0.00042 * E * s(M + 2 * F) + 0.00038 * E * s(M - 2 * F)
                 - 0.00024 * E * s(2 * Mp - M) - 0.00017 * s(Om) - 0.00007 * s(Mp + 2 * M)
                 + 0.00004 * s(2 * Mp - 2 * F) + 0.00004 * s(3 * M) + 0.00003 * s(Mp + M - 2 * F)
                 + 0.00003 * s(2 * Mp + 2 * F) - 0.00003 * s(Mp + M + 2 * F) + 0.00003 * s(Mp - M + 2 * F)
                 - 0.00002 * s(Mp - M - 2 * F) - 0.00002 * s(3 * Mp + M) + 0.00002 * s(4 * Mp))
    else:
        c = (-0.62801 * s(Mp) + 0.17172 * E * s(M) - 0.01183 * E * s(Mp + M) + 0.00862 * s(2 * Mp)
             + 0.00804 * s(2 * F) + 0.00454 * E * s(Mp - M) + 0.00204 * E * E * s(2 * M)
             - 0.00180 * s(Mp - 2 * F) - 0.00070 * s(Mp + 2 * F) - 0.00040 * s(3 * Mp)
             - 0.00034 * E * s(2 * Mp - M) + 0.00032 * E * s(M + 2 * F) + 0.00032 * E * s(M - 2 * F)
             - 0.00028 * E * E * s(Mp + 2 * M) + 0.00027 * E * s(2 * Mp + M) - 0.00017 * s(Om)
             - 0.00005 * s(Mp - M - 2 * F) + 0.00004 * s(2 * Mp + 2 * F) - 0.00004 * s(Mp + M + 2 * F)
             + 0.00004 * s(Mp - 2 * M) + 0.00003 * s(Mp + M - 2 * F) + 0.00003 * s(3 * M)
             + 0.00002 * s(2 * Mp - 2 * F) + 0.00002 * s(Mp - M + 2 * F) - 0.00002 * s(3 * Mp + M))
        W = (0.00306 - 0.00038 * E * math.cos(M) + 0.00026 * math.cos(Mp)
             - 0.00002 * math.cos(Mp - M) + 0.00002 * math.cos(Mp + M) + 0.00002 * math.cos(2 * F))
        c += W if frac == 0.25 else -W
    coeffs = [0.000325, 0.000165, 0.000164, 0.000126, 0.000110, 0.000062, 0.000060,
              0.000056, 0.000047, 0.000042, 0.000040, 0.000037, 0.000035, 0.000023]
    c += sum(cf * s(a * D2R) for cf, a in zip(coeffs, A))
    return JDE + c


def phases_between(jd_start, jd_end):
    """All four phases with JDE in [jd_start, jd_end]. Returns list of (name, jde)."""
    names = {0.0: "new", 0.25: "first quarter", 0.5: "full", 0.75: "last quarter"}
    # approximate k for start (Meeus 49.2): k = (year - 2000) * 12.3685
    y_start = 2000 + (jd_start - 2451545.0) / 365.25
    k0 = math.floor((y_start - 2000) * 12.3685) - 2
    out = []
    k = k0
    while True:
        for f in (0.0, 0.25, 0.5, 0.75):
            jde = moon_phase_jde(k + f)
            if jd_start <= jde <= jd_end:
                out.append((names[f], jde))
        if moon_phase_jde(k) > jd_end:
            break
        k += 1
    out.sort(key=lambda t: t[1])
    return out


def fmt_jd(jd, offset_hours=0.0):
    """Format a JD (UT) as Julian-calendar 'YYYY-MM-DD HH:MM' after adding an offset in hours."""
    y, m, d = cal_from_jd(jd + offset_hours / 24.0)
    day = int(math.floor(d))
    frac = d - day
    hh = int(frac * 24)
    mm = int(round((frac * 24 - hh) * 60))
    if mm == 60:
        hh, mm = hh + 1, 0
    return f"{y:04d}-{m:02d}-{day:02d} {hh:02d}:{mm:02d}"


if __name__ == "__main__":
    # ---- Positive controls: Meeus's own worked examples ----
    # Example 49.a: New Moon 1977 Feb 18, JDE 2443192.65118 (3h37m TD)
    k = -283
    jde = moon_phase_jde(k)
    print("Ex 49.a new moon 1977 Feb 18: JDE", round(jde, 5), "(book: 2443192.65118)")
    # Example 49.b: Last quarter 2044 Jan 21, JDE 2467636.49186 (23h48m TD)
    jde = moon_phase_jde(544.75)
    print("Ex 49.b last quarter 2044 Jan 21: JDE", round(jde, 5), "(book: 2467636.49186)")
    # Example 7.a: 1957 Oct 4.81 -> JD 2436116.31 ; Example 7.b: 333 Jan 27.5 (Julian) -> 1842713.0
    print("Ex 7.a JD 1957-10-04.81 =", jd_from_cal(1957, 10, 4.81), "(book: 2436116.31)")
    print("Ex 7.b JD 333-01-27.5 (Julian) =", jd_from_cal(333, 1, 27.5), "(book: 1842713.0)")
    print("Inverse 2436116.31 ->", cal_from_jd(2436116.31), "(book: 1957 10 4.81)")
    # Example 25.a: Sun 1992 Oct 13.0 TD -> apparent lon 199.90895 (low-accuracy method: 199.90988?), RA 13h13m31.4s, dec -7 47' 06"
    ra, dec, R, lam = sun_apparent(2448908.5)
    print("Ex 25.a sun 1992-10-13.0: lam", round(lam, 5), "ra(h)", round(ra / 15, 4), "dec", round(dec, 4), "R", round(R, 6),
          "(book low-acc: lam 199.90988, ra 13.2253h=198.38 deg, dec -7.7850)")
    # Example 12.a: mean sidereal time 1987 Apr 10 0h UT -> 13h10m46.3668s
    print("Ex 12.a GMST 1987-04-10 0h UT (h):", round(gmst_deg(2446895.5) / 15, 6), "(book: 13.179627)")
    # Weekday control: 29 May 1453 (Julian) was a Tuesday (fall of Constantinople)
    print("Weekday 1453-05-29 Julian:", weekday_from_jd(jd_from_cal(1453, 5, 29)), "(known: Tue)")
    print("Delta T 1462 (s):", round(delta_t_seconds(1462), 1), "; 1600:", round(delta_t_seconds(1600), 1), "(table ~120 s)")
