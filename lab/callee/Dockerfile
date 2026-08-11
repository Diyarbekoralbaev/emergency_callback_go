FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -qq && apt-get install -y -qq asterisk && \
    rm -rf /var/lib/apt/lists/* && \
    rm -f /etc/asterisk/pjsip.conf /etc/asterisk/extensions.conf
COPY pjsip.conf extensions.conf /etc/asterisk/
# rw config uchun asterisk useri
RUN chown -R asterisk:asterisk /etc/asterisk
CMD ["asterisk", "-f", "-U", "asterisk", "-G", "asterisk"]
