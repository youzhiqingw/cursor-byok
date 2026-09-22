# 发布镜像：仅承载 Windows 构建产物（zip / exe / sha256 / update.json）
# 构建上下文 = bin/release/<version>/ 目录；可用 docker create + docker cp 取出文件
FROM scratch
COPY . /release/