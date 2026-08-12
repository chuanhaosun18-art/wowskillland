# 临时验证脚本：产出 result.txt（含 DONE）并打印 DONE
import io

with io.open("result.txt", "w", encoding="utf-8") as f:
    f.write("DONE")
print("DONE")
