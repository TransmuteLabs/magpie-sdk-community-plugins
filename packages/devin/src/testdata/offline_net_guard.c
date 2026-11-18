#define _GNU_SOURCE
#include <arpa/inet.h>
#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <netdb.h>
#include <netinet/in.h>
#include <stdlib.h>
#include <sys/socket.h>
#include <unistd.h>

static void refused(void) {
    const char *path = getenv("DEVIN_TEST_NET_GUARD_LOG");
    if (path != NULL) {
        int descriptor = open(path, O_WRONLY | O_CREAT | O_APPEND, 0600);
        if (descriptor >= 0) {
            const char event[] = "blocked-nonloopback\n";
            (void)write(descriptor, event, sizeof(event) - 1);
            (void)close(descriptor);
        }
    }
}

int connect(int descriptor, const struct sockaddr *address, socklen_t length) {
    int allowed = address != NULL && address->sa_family == AF_UNIX;
    if (address != NULL && address->sa_family == AF_INET && length >= sizeof(struct sockaddr_in)) {
        const struct sockaddr_in *target = (const struct sockaddr_in *)address;
        allowed = (ntohl(target->sin_addr.s_addr) >> 24) == 127;
    }
    if (address != NULL && address->sa_family == AF_INET6 && length >= sizeof(struct sockaddr_in6)) {
        const struct sockaddr_in6 *target = (const struct sockaddr_in6 *)address;
        allowed = IN6_IS_ADDR_LOOPBACK(&target->sin6_addr) ||
            (IN6_IS_ADDR_V4MAPPED(&target->sin6_addr) && target->sin6_addr.s6_addr[12] == 127);
    }
    if (!allowed) {
        refused();
        errno = EPERM;
        return -1;
    }
    int (*original)(int, const struct sockaddr *, socklen_t) = dlsym(RTLD_NEXT, "connect");
    if (original == NULL) {
        errno = EPERM;
        return -1;
    }
    return original(descriptor, address, length);
}

int getaddrinfo(const char *host, const char *service, const struct addrinfo *hints, struct addrinfo **result) {
    struct in_addr ipv4;
    struct in6_addr ipv6;
    int allowed = host == NULL || inet_pton(AF_INET, host, &ipv4) == 1 ||
        inet_pton(AF_INET6, host, &ipv6) == 1;
    if (!allowed) {
        const char *name = "localhost";
        size_t index = 0;
        while (host[index] != '\0' && host[index] == name[index]) index++;
        allowed = host[index] == '\0' && name[index] == '\0';
    }
    if (!allowed) {
        refused();
        return EAI_FAIL;
    }
    int (*original)(const char *, const char *, const struct addrinfo *, struct addrinfo **) = dlsym(RTLD_NEXT, "getaddrinfo");
    if (original == NULL) return EAI_FAIL;
    return original(host, service, hints, result);
}
