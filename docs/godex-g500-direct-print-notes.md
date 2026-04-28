# GoDEX G500 direct USB print: chuqur texnik izoh

Bu hujjat GoDEX G500 printeriga oddiy matnli label chiqarish jarayonini
boshidan oxirigacha tushuntiradi. Maqsad faqat "qaysi buyruq ishladi"ni yozish
emas, balki nima uchun CUPS queue yetarli bo'lmagani, printer qizil statusda
nima sababdan turib qolgani, USB orqali printer bilan qanday gaplashilgani va
EZPL buyrug'i qanday qilib qog'ozga aylanganini izohlashdir.

Sinov natijasida quyidagi matnlar printerdan chiqdi:

- `TEST`
- `abdulfattox`
- `gandon`

Asosiy xulosa: printerga haqiqiy yechim CUPS queue orqali emas, USB bulk
endpointlarga bevosita EZPL buyruqlarini yuborish orqali topildi. Queue faqat
spooler edi; printer ichidagi media/sensor xatosi queue bilan hal bo'lmas edi.

## Qisqa natija

Ishlagan transport:

- USB vendor/product: `195f:0001`
- Printer: `GODEX INTERNATIONAL CO. G500`
- USB class: Printer, bidirectional
- OUT endpoint: `0x01`
- IN endpoint: `0x82`

Ishlagan printer tili:

- EZPL

Ishlagan minimal label formati:

```text
^Q25,3
^W50
^H10
^P1
^L
AC,20,20,1,1,0,0,TEST
E
```

Ishlashdan oldingi xato holati:

```text
02,00001
```

Tuzatilgandan keyingi sog'lom holat:

```text
00,00000
```

## Muammoni to'g'ri model qilish

Label printer oddiy "matn yuborilsa chiqadi" degan qurilma emas. Uni uchta
qatlamli tizim deb ko'rish kerak.

1. Transport qatlam

   Kompyuterdan printerga baytlar qanday boradi. Bizda bu USB orqali ishladi.
   CUPS ham shu qatlamga oxir-oqibat bayt yuboradi, lekin u o'rtada queue,
   backend va filter qo'shadi.

2. Protokol qatlam

   Printer kelgan baytlarni qaysi printer tili deb tushunadi. G500 EZPL, GEPL,
   GZPL kabi tillarni qo'llaydi. Biz ishlatgan buyruqlar EZPL sintaksisida.

3. Printer holati qatlam

   Printer qog'ozni ko'ryaptimi, label gap sensorini topyaptimi, direct thermal
   rejimidami, head yopiqmi, qizil status bormi. Agar shu qatlam xato holatda
   bo'lsa, protokol ham, queue ham to'g'ri bo'lsa-da, label chiqmasligi mumkin.

Shu sababli "queue'da job yo'qoldi" degani "printer bosdi" degani emas. Queue
jobni backendga topshirgan bo'lishi mumkin, printer esa sensor xatosi sababli
hech narsa chiqarmasligi mumkin.

## Nega CUPS queue bilan muammo hal bo'lmadi

Avval ikki xil CUPS yo'li sinab ko'rildi:

- `godex_g500_raw`
- `godex_g500_tpu`

`godex_g500_raw` jobni qabul qildi va queue bo'shab qoldi. Lekin fizik label
chiqmadi. Bu shuni bildiradi: CUPS jobni qabul qilgan, ammo printer tomonda
print bajarilganini isbotlamagan.

`godex_g500_tpu` TurboPrint backend orqali ochildi:

```text
tpu://GODEX/G500/SN=255109E1
```

Bu yo'lda job `printing` holatida osilib qoldi. Bu ham printer bilan backend
o'rtasida real javob muammosi borligini ko'rsatdi.

Shundan keyin asosiy diagnostika CUPS'dan USB darajasiga tushirildi. Bu muhim
burilish edi: endi spooler emas, printerning o'zi javob beradimi-yo'qmi
tekshirildi.

## USB darajadagi dalil

`lsusb -v -d 195f:0001` orqali printer descriptorlari ko'rildi. Muhim joylari:

```text
idVendor           0x195f GODEX INTERNATIONAL CO.
idProduct          0x0001 G500
bInterfaceClass         7 Printer
bInterfaceProtocol      2 Bidirectional
bEndpointAddress     0x01  EP 1 OUT
bEndpointAddress     0x82  EP 2 IN
```

Bu nimani anglatadi:

- `0x01` endpointiga yoziladi, ya'ni kompyuter printerga buyruq yuboradi.
- `0x82` endpointidan o'qiladi, ya'ni printer status javobini qaytaradi.
- `Bidirectional` bo'lgani uchun faqat "write-only printer" emas, status so'rab
  real javob olish mumkin.

PyUSB aynan shu endpointlar bilan ishlatildi.

## PyUSB nima qildi

Python kodi quyidagicha printer qurilmasini topdi:

```python
import usb.core
import usb.util

dev = usb.core.find(idVendor=0x195f, idProduct=0x0001)
```

Keyin active configuration olinib, OUT va IN endpointlar topildi:

```python
intf = dev.get_active_configuration()[(0, 0)]

ep_out = usb.util.find_descriptor(
    intf,
    custom_match=lambda e:
        usb.util.endpoint_direction(e.bEndpointAddress) == usb.util.ENDPOINT_OUT,
)

ep_in = usb.util.find_descriptor(
    intf,
    custom_match=lambda e:
        usb.util.endpoint_direction(e.bEndpointAddress) == usb.util.ENDPOINT_IN,
)
```

Praktik natija:

```text
eps 0x1 0x82
```

Bu `lsusb` bergan descriptor bilan mos tushdi. Demak, endi biz printerga queue
orqali emas, uning real USB endpointi orqali bayt yozayotgan edik.

## EZPL buyrug'i qanday o'qiladi

EZPL buyruqlari matnli buyruqlar bo'lib, odatda har bir buyruq `CRLF` bilan
tugatiladi:

```text
\r\n
```

Printerga yuborilgan minimal label:

```text
^Q25,3
^W50
^H10
^P1
^L
AC,20,20,1,1,0,0,TEST
E
```

Bu format ichida har bir satrning vazifasi bor.

`^Q25,3`

Label uzunligi va gap haqidagi sozlama. `25` label uzunligini bildiradi,
`3` gap qiymati sifatida ishlatilgan. Bizdagi real label shu kichik test uchun
yetarli bo'ldi.

`^W50`

Label kengligi. GoDEX EZPL hujjatida `^W` label width sozlamasi sifatida
keladi. Bu test uchun 50 qiymati ishladi.

`^H10`

Print darkness. Termal bosishda qoraytirish darajasi. Juda past bo'lsa yozuv
xira chiqadi, juda baland bo'lsa qog'oz ortiqcha qizishi mumkin.

`^P1`

Bitta label bosish. `^P` print count sifatida ishlatiladi.

`^L`

Label formatting mode boshlanishi. Bundan keyin keladigan chizish, text,
barcode va boshqa obyektlar label ichidagi obyekt sifatida qabul qilinadi.

`AC,20,20,1,1,0,0,TEST`

Bu text obyekt. EZPL outline'da `At,x,y,x_mul,y_mul,gap,rotationInverse,data`
ko'rinishidagi text command bor. Bu yerda:

- `A` text command oilasi
- `C` font turi
- `20,20` label ichidagi x/y koordinata
- `1,1` x va y bo'yicha kattalashtirish
- `0` harflar orasidagi gap
- `0` rotation/inverse parametri
- `TEST` bosiladigan data

`E`

Formatni tugatish va labelni print qilish. `E` kelmaguncha printer obyektlarni
format sifatida yig'ib turadi; `E` esa shu formatni yakunlab bosishni
boshlatadi.

## Direct thermal rejimi

Printerlarda ikki asosiy termal rejim bor:

- Direct thermal: qog'ozning o'zi issiqlikka reaksiya qiladi, ribbon kerak emas.
- Thermal transfer: ribbon orqali bo'yoq labelga o'tadi.

Bizning holatda direct thermal kerak edi, shuning uchun:

```text
^AD
```

yuborildi.

G500 user manual error alert qismida ribbon bilan bog'liq xatolarda direct
thermal mode tekshirish kerakligi aytiladi. Shu sababli `^AD` recovery
ketma-ketligiga qo'shildi: printer ribbon kutib qolmasligi kerak.

## Statusni o'qish

Printerga status so'rovi yuborildi:

```text
~S,STATUS
```

Bunga printer avval shunday javob berdi:

```text
02,00001
```

Bu holat printer panelidagi qizil status bilan birga kuzatildi. G500 user
manualidagi Error Alerts bo'limida feed/media muammolari uchun quyidagi sabablar
keltiriladi:

- label sensor qog'ozni topmayapti
- qog'oz tugagan
- print medium rubber roll atrofida tiqilib qolgan
- sensor label orasidagi gap yoki black markni topmayapti
- sensorni qayta reset/autodetect qilish kerak

Shu sababli `02,00001` amalda media/sensor/feed muammosi sifatida talqin
qilindi. Muhim nuqta: xato queue'da emas edi, printer ichki holatida edi.

Recovery'dan keyin status:

```text
00,00000
```

Bu printer buyruqlarni normal qabul qilishga tayyor holatga qaytganini
ko'rsatdi.

## Recovery ketma-ketligi

Printer qizil statusda turganda ishlagan amaliy ketma-ketlik:

```text
~S,ESG
^AD
^XSET,IMMEDIATE,1
^XSET,ACTIVERESPONSE,1
~Z
~S,CANCEL
~S,SENSOR
~S,STATUS
```

Har birining vazifasi:

`~S,ESG`

Command language holatini normallashtirish uchun yuborildi. G500 bir nechta
printer tilini qo'llaydi, shuning uchun diagnostika boshida printer kelayotgan
matnni EZPL oilasidagi buyruq deb ko'rishi kerak edi.

`^AD`

Direct thermal mode. Bu ribbon bilan bog'liq noto'g'ri kutishni chetlab o'tadi.

`^XSET,IMMEDIATE,1`

Immediate response yoqildi. Status so'rovlarida javob olishni tezlashtiradi.

`^XSET,ACTIVERESPONSE,1`

Printer error/status javoblarini faol qaytarishi uchun yoqildi.

`~Z`

Printer reset. Bu queue cancel emas, printer ichidagi holatni qayta boshlash.

`~S,CANCEL`

Printer ichida qolgan joriy operatsiyani bekor qilish. Agar oldingi print job
yarim holatda qolgan bo'lsa, uni tozalashga yordam beradi.

`~S,SENSOR`

Auto sensing. Printer label balandligi va gap/mark sensor holatini qayta
o'lchaydi. Bizdagi `02,00001` muammosini yechishda eng muhim qadamlardan biri
shu bo'ldi.

`~S,STATUS`

Recovery natijasini tekshirish. Bu qadamdan keyin `00,00000` qaytdi.

## To'liq ishlagan PyUSB oqimi

Quyidagi skript queue ishlatmaydi. U to'g'ridan-to'g'ri `195f:0001` G500
qurilmasini topadi, endpointlarga yozadi, statusni o'qiydi va bitta label
chiqaradi.

```python
import sys
import time

import usb.core
import usb.util

VID = 0x195F
PID = 0x0001


def find_printer():
    dev = usb.core.find(idVendor=VID, idProduct=PID)
    if dev is None:
        raise RuntimeError("GoDEX G500 not found")

    if dev.is_kernel_driver_active(0):
        try:
            dev.detach_kernel_driver(0)
        except Exception:
            pass

    dev.set_configuration()
    intf = dev.get_active_configuration()[(0, 0)]

    ep_out = usb.util.find_descriptor(
        intf,
        custom_match=lambda e:
            usb.util.endpoint_direction(e.bEndpointAddress)
            == usb.util.ENDPOINT_OUT,
    )
    ep_in = usb.util.find_descriptor(
        intf,
        custom_match=lambda e:
            usb.util.endpoint_direction(e.bEndpointAddress)
            == usb.util.ENDPOINT_IN,
    )

    if ep_out is None or ep_in is None:
        raise RuntimeError("USB endpoints not found")

    return dev, ep_out, ep_in


def send(dev, ep_out, ep_in, command, read=False, pause=0.12):
    if isinstance(command, str):
        command = command.encode("ascii")
    if not command.endswith(b"\r\n"):
        command += b"\r\n"

    dev.write(ep_out.bEndpointAddress, command, timeout=2000)
    time.sleep(pause)

    if not read:
        return ""

    try:
        data = dev.read(ep_in.bEndpointAddress, 512, timeout=1200)
        return bytes(data).decode("latin1", "replace").strip()
    except Exception:
        return ""


def recover(dev, ep_out, ep_in):
    for command in [
        "~S,ESG",
        "^AD",
        "^XSET,IMMEDIATE,1",
        "^XSET,ACTIVERESPONSE,1",
        "~Z",
        "~S,CANCEL",
        "~S,SENSOR",
    ]:
        send(dev, ep_out, ep_in, command, pause=0.3)

    return send(dev, ep_out, ep_in, "~S,STATUS", read=True)


def print_text(dev, ep_out, ep_in, text):
    for command in [
        "~S,ESG",
        "^AD",
        "^XSET,IMMEDIATE,1",
        "^XSET,ACTIVERESPONSE,1",
        "^Q25,3",
        "^W50",
        "^H10",
        "^P1",
        "^L",
        f"AC,20,20,1,1,0,0,{text}",
        "E",
    ]:
        send(dev, ep_out, ep_in, command)

    time.sleep(1.0)
    return send(dev, ep_out, ep_in, "~S,STATUS", read=True)


if __name__ == "__main__":
    text = sys.argv[1] if len(sys.argv) > 1 else "TEST"
    dev, ep_out, ep_in = find_printer()

    status = send(dev, ep_out, ep_in, "~S,STATUS", read=True)
    if status and not status.startswith("00,"):
        status = recover(dev, ep_out, ep_in)

    final_status = print_text(dev, ep_out, ep_in, text)
    print(final_status)
```

## Diagnostika logikasi

Jarayon quyidagi qaror daraxti bilan yurdi:

1. CUPS queue job qabul qiladimi?

   Ha, qabul qildi. Lekin label chiqmadi. Demak bu hali printer ishladi degani
   emas.

2. USB qurilma ko'rinyaptimi?

   Ha, `195f:0001 GODEX G500` ko'rindi.

3. USB interface bidirectionalmi?

   Ha, OUT `0x01`, IN `0x82`. Demak status o'qish mumkin.

4. Printer status javob beradimi?

   Ha, `~S,STATUS` javob berdi.

5. Status normalmi?

   Avval yo'q: `02,00001`.

6. Xato nimaga o'xshaydi?

   Panel qizil edi va G500 manualidagi feed/media/sensor xatolari bilan mos
   tushdi.

7. Sensor/reset recovery ishladimi?

   Ha, status `00,00000` bo'ldi.

8. Minimal EZPL label ishladimi?

   Ha, `TEST` chiqdi. Keyin `abdulfattox` va `gandon` ham chiqdi.

## Nega `50,00001` ko'rinishi mumkin

Ba'zi printlardan keyin status `50,00001` ko'rindi, lekin label chiqdi. Bu
amaliy kuzatuv shuni ko'rsatadi: har bir non-zero statusni darhol "print
bajarilmadi" deb talqin qilib bo'lmaydi. Printer labelni bosib bo'lganidan
keyin ham ogohlantirish yoki ichki holat bayrog'ini qaytarishi mumkin.

Shu sababli amaliy tekshiruv ikki qismdan iborat bo'lishi kerak:

- fizik label chiqdimi
- keyingi `~S,STATUS` printer ishlashiga to'sqinlik qiladigan holatdami

Agar printer qizil statusda qolsa yoki keyingi print chiqmasa, recovery
ketma-ketligi qayta bajariladi.

## Operator uchun qisqa tartib

Printer qizil bo'lsa:

1. Label roll to'g'ri yotganini tekshirish.
2. Print mechanism/head yopilganini tekshirish.
3. Direct thermal ishlatilayotgan bo'lsa ribbon talab qilinmasligini tekshirish.
4. `~Z`, `~S,CANCEL`, `~S,SENSOR` ketma-ketligini yuborish.
5. `~S,STATUS` bilan `00,00000` holatini kutish.
6. Keyin minimal EZPL label yuborish.

Minimal print:

```text
^Q25,3
^W50
^H10
^P1
^L
AC,20,20,1,1,0,0,hello
E
```

## Muhim ehtiyot nuqtalari

CUPS queue holati bilan printer holatini aralashtirmaslik kerak. Queue
`ready` desa ham, printer paneli qizil bo'lishi mumkin.

Printer statusini har doim printerning o'zidan so'rash kerak. Bizda bu
`~S,STATUS` orqali USB IN endpointdan o'qildi.

`E` buyrug'i bo'lmasa, label bosilmaydi. `^L` formatni boshlaydi, `E` esa
formatni yakunlab printni trigger qiladi.

Media sensor xatosida bir xil label buyrug'ini yuz marta yuborish foyda
bermaydi. Avval sensor holatini tuzatish kerak.

Direct thermal printerda `^AD` muhim. Agar printer thermal transfer/ribbon
rejimida qolgan bo'lsa, ribbon yo'qligidan xato ko'rsatishi mumkin.

## Manbalar

- GoDEX EZPL Programmer's Manual: `https://www.manualsdir.com/manuals/736893/godex-ezpl.html`
- EZPL command outline: `^Q`, `^W`, `^XSET`, `~S,SENSOR`, `~S,STATUS`, `~V`, `~Z`, `At...`, `E`
- GoDEX G500 User Manual, Label Size Calibration and Self Test Page: `https://www.manualslib.com/manual/954453/Godex-G500.html?page=20`
- GoDEX G500 User Manual, Error Alerts: `https://www.manualslib.com/manual/954453/Godex-G500.html?page=21`

## Yakuniy xulosa

Bu muammo printerga matn yuborish muammosi emas edi. Asl muammo printer
sensor/media holatida edi. CUPS queue jobni qabul qilishi mumkin edi, lekin
printer qizil holatda bo'lgani uchun fizik print chiqmasdi.

Ishlagan yechim:

1. USB descriptor orqali printer endpointlarini topish.
2. PyUSB bilan OUT endpointga EZPL yuborish.
3. IN endpointdan `~S,STATUS` javobini o'qish.
4. `02,00001` holatini media/sensor/feed muammosi deb ajratish.
5. `~Z`, `~S,CANCEL`, `~S,SENSOR` bilan printer holatini tiklash.
6. `00,00000` bo'lgandan keyin minimal EZPL label yuborish.

Natijada printer real label chiqardi.
