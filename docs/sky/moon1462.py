# Moon phases and moonrise for the slice nights, Targoviste (44.93N 25.46E), Julian-calendar dates.
# PyEphem (analytic ephemeris; interprets dates before 1582-10-15 as Julian calendar). VERIFY against JPL in the C2 pass.
import ephem
LMT = 25.46/15.0/24.0  # local mean solar time offset from UT, in days
print("full moon (eclipse anchor):", ephem.next_full_moon('1462/6/10'))
print("last quarter:", ephem.next_last_quarter_moon('1462/6/12'))
print("new moon:", ephem.next_new_moon('1462/6/12'))
print("next full:", ephem.next_full_moon('1462/6/13'))
obs = ephem.Observer(); obs.lat, obs.lon, obs.elevation = '44.93', '25.46', 280
moon, sun = ephem.Moon(), ephem.Sun()
print("\nnight of | sunset | moonrise | illum | sunrise (local mean solar time)")
for day in range(16, 25):
    obs.date = ephem.Date(f'1462/6/{day} 09:00'); ss = obs.next_setting(sun)
    obs.date = ss; sr = obs.next_rising(sun); mr = obs.next_rising(moon)
    moon.compute(ephem.Date(ss + 0.2))
    print(f"Jun {day:2d} | {ephem.Date(ss+LMT)} | {ephem.Date(mr+LMT)} | {moon.phase:4.0f}% | {ephem.Date(sr+LMT)}")
