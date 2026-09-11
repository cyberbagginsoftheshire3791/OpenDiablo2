# Daylight / darkness at Targoviste (44.93N, 25.46E) for Julian-calendar dates in 1462.
# NOAA solar position algorithm (Meeus). Durations are timezone-independent; times are local mean solar time.
import math

LAT, LON = 44.93, 25.46

def jdn_julian(y, m, d):
    # Julian Day Number for a JULIAN-calendar date
    a = (14 - m)//12
    yy = y + 4800 - a
    mm = m + 12*a - 3
    return d + (153*mm + 2)//5 + 365*yy + yy//4 - 32083

def jdn_gregorian(y, m, d):
    a = (14 - m)//12
    yy = y + 4800 - a
    mm = m + 12*a - 3
    return d + (153*mm + 2)//5 + 365*yy + yy//4 - yy//100 + yy//400 - 32045

def solar_params(jd):
    T = (jd - 2451545.0)/36525.0
    L0 = (280.46646 + T*(36000.76983 + T*0.0003032)) % 360
    M = 357.52911 + T*(35999.05029 - 0.0001537*T)
    e = 0.016708634 - T*(0.000042037 + 0.0000001267*T)
    Mr = math.radians(M)
    C = (1.914602 - T*(0.004817 + 0.000014*T))*math.sin(Mr) + (0.019993 - 0.000101*T)*math.sin(2*Mr) + 0.000289*math.sin(3*Mr)
    true_long = L0 + C
    omega = 125.04 - 1934.136*T
    lam = true_long - 0.00569 - 0.00478*math.sin(math.radians(omega))
    eps0 = 23 + (26 + (21.448 - T*(46.815 + T*(0.00059 - T*0.001813)))/60)/60
    eps = eps0 + 0.00256*math.cos(math.radians(omega))
    decl = math.degrees(math.asin(math.sin(math.radians(eps))*math.sin(math.radians(lam))))
    y = math.tan(math.radians(eps/2))**2
    L0r = math.radians(L0)
    eqtime = 4*math.degrees(y*math.sin(2*L0r) - 2*e*math.sin(Mr) + 4*e*y*math.sin(Mr)*math.cos(2*L0r) - 0.5*y*y*math.sin(4*L0r) - 1.25*e*e*math.sin(2*Mr))
    return decl, eqtime

def hour_angle(lat, decl, alt):
    # returns hour angle in degrees for sun altitude 'alt' (deg), or None if never reached
    latr, dr = math.radians(lat), math.radians(decl)
    cosH = (math.sin(math.radians(alt)) - math.sin(latr)*math.sin(dr))/(math.cos(latr)*math.cos(dr))
    if cosH < -1: return 180.0   # always above
    if cosH > 1: return 0.0      # never above
    return math.degrees(math.acos(cosH))

def report(label, jd):
    decl, eq = solar_params(jd + 0.5)
    out = {"label": label, "decl": decl}
    for name, alt in [("sun", -0.833), ("civil", -6), ("nautical", -12), ("astro", -18)]:
        H = hour_angle(LAT, decl, alt)
        out[name] = 2*H/15  # hours above that altitude
    # local mean solar time of sunrise/sunset (solar noon = 12:00 - eqtime)
    noon = 12 - eq/60
    H = hour_angle(LAT, decl, -0.833)
    out["sunrise"] = noon - H/15
    out["sunset"] = noon + H/15
    out["midnight_alt"] = -(90 - LAT + decl) if decl >= 0 else -(90 - LAT + decl)
    return out

def fmt_h(h):
    hh = int(h); mm = int(round((h - hh)*60))
    if mm == 60: hh += 1; mm = 0
    return f"{hh:02d}:{mm:02d}"

dates = [
    ("1462 Jun 12 (Jul.) — partial eclipse / full moon", jdn_julian(1462,6,12)),
    ("1462 Jun 17 (Jul.) — morning after the Night Attack", jdn_julian(1462,6,17)),
    ("1462 Jun 22 (Jul.) — Mehmed withdraws", jdn_julian(1462,6,22)),
    ("1462 Jul 1 (Jul.)", jdn_julian(1462,7,1)),
    ("1462 Jul 15 (Jul.)", jdn_julian(1462,7,15)),
    ("1462 Aug 1 (Jul.)", jdn_julian(1462,8,1)),
    ("1462 Sep 1 (Jul.)", jdn_julian(1462,9,1)),
    ("1462 Oct 1 (Jul.)", jdn_julian(1462,10,1)),
    ("1462 Nov 1 (Jul.)", jdn_julian(1462,11,1)),
    ("1462 Nov 26 (Jul.) — St Andrew's Eve (Nov 30 feast: eve is Nov 29)", jdn_julian(1462,11,29)),
    ("1462 Dec 12 (Jul.) — near solstice", jdn_julian(1462,12,12)),
    ("1462 Dec 25 (Jul.)", jdn_julian(1462,12,25)),
]
print("Julian date check: 1462 Jun 17 Julian -> JDN", jdn_julian(1462,6,17), "; Gregorian equivalent offset days:", jdn_julian(1462,6,17) - jdn_gregorian(1462,6,17))
print()
print(f"{'date':62s} {'decl':>6s} {'sunrise':>8s} {'sunset':>7s} {'day':>6s} {'night':>6s} {'<-6':>6s} {'<-12':>6s} {'<-18':>6s}")
for label, jd in dates:
    r = report(label, jd)
    day = r['sun']; night = 24 - day
    below6 = 24 - r['civil']; below12 = 24 - r['nautical']; below18 = 24 - r['astro']
    print(f"{label:62s} {r['decl']:6.1f} {fmt_h(r['sunrise']):>8s} {fmt_h(r['sunset']):>7s} {day:6.2f} {night:6.2f} {below6:6.2f} {below12:6.2f} {below18:6.2f}")
