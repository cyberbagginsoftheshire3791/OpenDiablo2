# Calendar cross-checks for the slice window (all civil dates JULIAN calendar, as contemporaries used).
def jdn_julian(y, m, d):
    a = (14 - m)//12; yy = y + 4800 - a; mm = m + 12*a - 3
    return d + (153*mm + 2)//5 + 365*yy + yy//4 - 32083

def julian_from_jdn(J):
    # inverse for Julian calendar (Richards / Meeus)
    c = J + 32082
    d = (4*c + 3)//1461
    e = c - (1461*d)//4
    m = (5*e + 2)//153
    day = e - (153*m + 2)//5 + 1
    month = m + 3 - 12*(m//10)
    year = d - 4800 + m//10
    return year, month, day

WD = ["Sun","Mon","Tue","Wed","Thu","Fri","Sat"]
def weekday(J): return WD[(J+1) % 7]

# 1. Weekday sanity anchors
print("Fall of Constantinople 29 May 1453 (known Tuesday):", weekday(jdn_julian(1453,5,29)))
print("Night Attack morning after, 17 Jun 1462:", weekday(jdn_julian(1462,6,17)))
for d in range(12, 30):
    J = jdn_julian(1462,6,d)
    print(f"  1462-06-{d:02d} {weekday(J)}")

# 2. Orthodox (Julian) Easter 1462 — Meeus Julian algorithm
Y = 1462
a = Y % 4; b = Y % 7; c = Y % 19
d = (19*c + 15) % 30
e = (2*a + 4*b - d + 34) % 7
month = (d + e + 114)//31
day = ((d + e + 114) % 31) + 1
E = jdn_julian(Y, month, day)
print("\nJulian Easter 1462:", julian_from_jdn(E), weekday(E))
print("Pentecost:", julian_from_jdn(E+49), weekday(E+49))
print("All Saints Sunday:", julian_from_jdn(E+56), weekday(E+56))
print("Apostles' Fast begins:", julian_from_jdn(E+57), weekday(E+57), "-> ends 28 Jun; feast Sts Peter & Paul 29 Jun")

# 3. Tabular Islamic calendar (civil epoch 1948440) — Ramadan 866 AH
def jdn_islamic(y, m, d):
    return (11*y + 3)//30 + 354*y + 30*m - (m - 1)//2 + d + 1948440 - 385
print("\nCheck: 1 Muharram 1 AH ->", julian_from_jdn(jdn_islamic(1,1,1)), "(expect 622-07-16 Julian)")
r1 = jdn_islamic(866, 9, 1); r30 = jdn_islamic(866, 9, 30); eid = jdn_islamic(866, 10, 1)
print("1 Ramadan 866 AH ->", julian_from_jdn(r1), weekday(r1))
print("30 Ramadan 866 AH ->", julian_from_jdn(r30), weekday(r30))
print("1 Shawwal 866 AH (Eid al-Fitr) ->", julian_from_jdn(eid), weekday(eid))

# 4. Moon phases from the 12 Jun 1462 eclipse anchor (full moon), mean synodic month
syn = 29.530589
full = jdn_julian(1462,6,12) + 0.5   # anchor to the day; hour unknown here (VERIFY)
print("\nMoon (from eclipse anchor, +/- 1 day):")
for k, name in [(0.25,"last quarter"),(0.5,"new moon"),(0.75,"first quarter"),(1.0,"full moon"),(1.25,"last quarter"),(1.5,"new moon"),(2.0,"full moon")]:
    J = full + k*syn
    print(f"  {name:14s} ~ {julian_from_jdn(int(J))}")
